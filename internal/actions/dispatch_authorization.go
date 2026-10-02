package actions

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func (d *Dispatcher) revalidateAuthorization(ctx context.Context, leased leasedCommand) error {
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		return d.revalidateAuthorizationTx(ctx, tx, leased)
	}); err != nil {
		return fmt.Errorf("revalidate dispatch authorization: %w", err)
	}
	return nil
}

// revalidateAuthorizationTx re-proves, immediately before the effector call,
// that the leased command is still authorized: the lease is live, the
// command, intent, and decision documents still match their digests and
// ledger rows, the intent is approved, unexpired, and bound to the current
// Situation version, interlocks and R2 approvals still hold, and the policy
// digest is current. It then refreshes the lease.
func (d *Dispatcher) revalidateAuthorizationTx(ctx context.Context, tx *sql.Tx, leased leasedCommand) error {
	if err := d.assertRuntimeOwner(ctx, tx); err != nil {
		return err
	}
	var leaseValid int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM outbox WHERE outbox_id = ? AND status = 'leased' AND lease_owner = ? AND lease_until > ?`, leased.OutboxID, leased.LeaseOwner, formatTime(d.clk.Now())).Scan(&leaseValid); err != nil {
		return fmt.Errorf("dispatch lease is no longer active: %w", err)
	}
	records, err := loadAuthorizationRecords(ctx, tx, leased.Command.CommandID)
	if err != nil {
		return err
	}
	commandDocument, err := records.verifiedCommand()
	if err != nil {
		return err
	}
	if !records.approvedForIntent() {
		return errors.New("command is no longer approved for its intent")
	}
	if d.interlock != nil {
		if err := d.interlock.Assert(ctx, tx, records.commandTenant, records.commandTarget, records.intentRisk); err != nil {
			return fmt.Errorf("interlock rejected command: %w", err)
		}
	}
	if err := records.checkApproval(d.clk.Now()); err != nil {
		return err
	}
	if !records.current() {
		return errors.New("command authorization is stale")
	}
	if err := records.checkIntent(d.clk.Now()); err != nil {
		return err
	}
	if err := checkPolicyDigest(ctx, tx, commandDocument, records.intentID); err != nil {
		return err
	}
	if err := records.checkDecision(); err != nil {
		return err
	}
	return d.refreshLease(ctx, tx, leased)
}

// authorizationRecords is every ledger row a command's authority rests on.
type authorizationRecords struct {
	commandID, commandTenant, commandIntent, commandRoute, commandTarget string
	commandJSON, commandSHA, commandIdempotency                          []byte

	intentTenant, intentID, decisionID, intentSituation string
	intentType, intentRisk, intentExpires, policyStatus string
	intentVersion                                       int
	intentJSON, intentSHA                               []byte

	approvedApprovalID, approvedApprovalExpiry sql.NullString

	validationStatus, decisionSituation, episodeID string
	decisionVersion                                int
	decisionJSON, decisionSHA                      []byte

	episodeTenant, episodeSituation, episodeLifecycle string
	episodeVersion                                    int

	situationTenant string
	currentVersion  int
}

func loadAuthorizationRecords(ctx context.Context, tx *sql.Tx, commandID string) (authorizationRecords, error) {
	r := authorizationRecords{commandID: commandID}
	if err := tx.QueryRowContext(ctx, `
			SELECT c.tenant_id, c.intent_id, c.effector_route, c.normalized_target,
			       c.idempotency_key, c.command_json, c.command_sha256,
			       i.tenant_id, i.intent_id, i.decision_id, i.situation_id,
			       i.situation_version, i.intent_type, i.risk_class, i.intent_json,
			       i.intent_sha256, i.expires_at, i.policy_status,
			       (SELECT a.approval_id FROM approvals a WHERE a.intent_id = i.intent_id AND a.status = 'approved' ORDER BY a.decided_at DESC LIMIT 1),
			       (SELECT a.expires_at FROM approvals a WHERE a.intent_id = i.intent_id AND a.status = 'approved' ORDER BY a.decided_at DESC LIMIT 1),
			       d.validation_status, d.raw_json, d.decision_sha256, d.situation_id, d.situation_version, d.episode_id,
			       e.tenant_id, e.situation_id, e.situation_version, e.lifecycle_status,
			       s.tenant_id, s.current_version
			FROM commands c
			JOIN intents i ON i.intent_id = c.intent_id
			JOIN decisions d ON d.decision_id = i.decision_id
			JOIN episodes e ON e.episode_id = d.episode_id
			JOIN situations s ON s.situation_id = i.situation_id
			WHERE c.command_id = ? AND c.status = 'dispatching'`, commandID).Scan(
		&r.commandTenant, &r.commandIntent, &r.commandRoute, &r.commandTarget,
		&r.commandIdempotency, &r.commandJSON, &r.commandSHA,
		&r.intentTenant, &r.intentID, &r.decisionID, &r.intentSituation,
		&r.intentVersion, &r.intentType, &r.intentRisk, &r.intentJSON,
		&r.intentSHA, &r.intentExpires, &r.policyStatus,
		&r.approvedApprovalID, &r.approvedApprovalExpiry,
		&r.validationStatus, &r.decisionJSON, &r.decisionSHA, &r.decisionSituation, &r.decisionVersion, &r.episodeID,
		&r.episodeTenant, &r.episodeSituation, &r.episodeVersion, &r.episodeLifecycle,
		&r.situationTenant, &r.currentVersion); err != nil {
		return authorizationRecords{}, fmt.Errorf("load authorization records: %w", err)
	}
	return r, nil
}

// verifiedCommand decodes the command document and requires it to be
// schema-valid, digest-bound, and identical to its ledger columns.
func (r authorizationRecords) verifiedCommand() (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(r.commandJSON, &document); err != nil || contractsv1.Validate(contractsv1.SchemaCommand, document) != nil || !verifyDigest(canonicaljson.DomainCommand, document, r.commandSHA) ||
		documentString(document, "command_id") != r.commandID || documentString(document, "intent_id") != r.commandIntent ||
		documentString(document, "tenant_id") != r.commandTenant || documentString(document, "effector_route") != r.commandRoute ||
		documentString(document, "normalized_target") != r.commandTarget || !bytes.Equal(r.commandIdempotency, mustDigest(document, "idempotency_key")) {
		return nil, errors.New("command ledger identity mismatch")
	}
	return document, nil
}

func (r authorizationRecords) approvedForIntent() bool {
	return r.commandTenant == r.intentTenant && r.commandIntent == r.intentID && r.commandRoute == r.intentType &&
		r.policyStatus == "approved" && r.validationStatus == "accepted"
}

// checkApproval requires an unexpired human approval for an R2 intent.
func (r authorizationRecords) checkApproval(now time.Time) error {
	if r.intentRisk != "R2" {
		return nil
	}
	if !r.approvedApprovalID.Valid {
		return errors.New("approved intent has no approved approval record")
	}
	approvalExpires, err := time.Parse(time.RFC3339Nano, r.approvedApprovalExpiry.String)
	if err != nil || !approvalExpires.After(now) {
		return errors.New("approval is expired")
	}
	return nil
}

// current reports whether the decision, episode, and live Situation all still
// match the intent's tenant and Situation version.
func (r authorizationRecords) current() bool {
	return r.episodeTenant == r.intentTenant && r.situationTenant == r.intentTenant &&
		r.decisionSituation == r.intentSituation && r.decisionVersion == r.intentVersion &&
		r.episodeSituation == r.intentSituation && r.episodeVersion == r.intentVersion &&
		(r.episodeLifecycle == "concluded" || r.episodeLifecycle == "closed") &&
		r.currentVersion == r.intentVersion
}

// checkIntent requires an unexpired, digest-bound intent document that
// matches its ledger row.
func (r authorizationRecords) checkIntent(now time.Time) error {
	expiresAt, err := time.Parse(time.RFC3339Nano, r.intentExpires)
	if err != nil || !expiresAt.After(now) {
		return errors.New("intent authorization is expired")
	}
	var document map[string]any
	if err := json.Unmarshal(r.intentJSON, &document); err != nil || contractsv1.Validate(contractsv1.SchemaIntent, document) != nil || !verifyIntentDigest(document, r.intentSHA) {
		return errors.New("intent authorization is invalid")
	}
	if documentString(document, "intent_id") != r.intentID || documentString(document, "decision_id") != r.decisionID ||
		documentString(document, "tenant_id") != r.intentTenant || documentString(document, "situation_id") != r.intentSituation ||
		documentInt(document, "situation_version") != r.intentVersion || documentString(document, "type") != r.intentType ||
		documentString(document, "risk_class") != r.intentRisk {
		return errors.New("intent authorization identity mismatch")
	}
	return nil
}

// checkDecision requires a digest-bound decision document that matches the
// intent's episode and Situation version.
func (r authorizationRecords) checkDecision() error {
	var document map[string]any
	if err := json.Unmarshal(r.decisionJSON, &document); err != nil || contractsv1.Validate(contractsv1.SchemaDecision, document) != nil || !verifyDigest(canonicaljson.DomainDecision, document, r.decisionSHA) {
		return errors.New("decision authorization is invalid")
	}
	if documentString(document, "decision_id") != r.decisionID || documentString(document, "episode_id") != r.episodeID ||
		documentString(document, "situation_id") != r.intentSituation || documentInt(document, "situation_version") != r.intentVersion {
		return errors.New("decision authorization identity mismatch")
	}
	return nil
}

// checkPolicyDigest requires a command that names a policy digest to match
// the latest approving policy evaluation of its intent.
func checkPolicyDigest(ctx context.Context, tx *sql.Tx, commandDocument map[string]any, intentID string) error {
	commandPolicyDigest := documentString(commandDocument, "policy_digest")
	if commandPolicyDigest == "" {
		return nil
	}
	var evaluatedPolicyDigest string
	if err := tx.QueryRowContext(ctx, `
		SELECT policy_digest FROM policy_evaluations
		WHERE intent_id = ? AND result = 'approved'
		ORDER BY evaluated_at DESC LIMIT 1`, intentID).Scan(&evaluatedPolicyDigest); err != nil {
		return fmt.Errorf("load approved policy digest: %w", err)
	}
	if commandPolicyDigest != evaluatedPolicyDigest {
		return errors.New("command policy digest is stale")
	}
	return nil
}

func (d *Dispatcher) refreshLease(ctx context.Context, tx *sql.Tx, leased leasedCommand) error {
	refreshNow := d.clk.Now()
	refresh, err := tx.ExecContext(ctx, `UPDATE outbox SET lease_until = ? WHERE outbox_id = ? AND status = 'leased' AND lease_owner = ? AND lease_until > ?`, formatTime(refreshNow.Add(d.leaseFor)), leased.OutboxID, leased.LeaseOwner, formatTime(refreshNow))
	if err != nil {
		return fmt.Errorf("refresh dispatch lease: %w", err)
	}
	if count, err := refresh.RowsAffected(); err != nil || count != 1 {
		return fmt.Errorf("refresh dispatch lease lost ownership")
	}
	return nil
}
