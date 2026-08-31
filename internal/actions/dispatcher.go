// Package actions owns the effect boundary after policy approval.
package actions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Command is the validated, policy-approved input to an effector.
type Command struct {
	CommandID        string
	IntentID         string
	TenantID         string
	EffectorRoute    string
	NormalizedTarget string
	IdempotencyKey   string
	PolicyDigest     string
	NotBeforeMonoUS  int64
	Payload          map[string]any
}

// Effect is the provider response. An effector must return UnknownOutcomeError
// when it cannot establish whether the provider applied the effect. Set
// VerificationPending when the provider only acknowledged transport receipt;
// independent feedback must then establish physical success.
type Effect struct {
	ProviderResult      map[string]any
	ObservedEffect      map[string]any
	VerificationPending bool
}

// Effector is the only interface allowed to cross from the action plane into
// an external system. Implementations must honor Command.IdempotencyKey.
type Effector interface {
	Dispatch(context.Context, Command) (Effect, error)
}

// DeviceStateVerifier verifies a device-backed command with one fresh state
// query. An empty final status means the command is not device-backed.
type DeviceStateVerifier interface {
	VerifyDeviceCommand(context.Context, Command) (finalStatus string, evidence map[string]any, err error)
}

// Authorization is the final runtime authorization check passed to a
// concrete effector. The check must run immediately before the effect is
// accepted by that effector.
type Authorization struct {
	Check func(context.Context) error
}

// AuthorizedEffector is required when the dispatcher has a live interlock.
// It closes the validation-to-acceptance gap at the concrete effect boundary.
type AuthorizedEffector interface {
	Effector
	DispatchAuthorized(context.Context, Command, Authorization) (Effect, error)
}

// UnknownOutcomeError means the request may have reached the provider, so the
// dispatcher records reconciliation as required and never blindly retries it.
type UnknownOutcomeError struct{ Err error }

func (e *UnknownOutcomeError) Error() string {
	if e == nil || e.Err == nil {
		return "action outcome is unknown"
	}
	return "action outcome is unknown: " + e.Err.Error()
}

func (e *UnknownOutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsUnknownOutcome reports whether err requires reconciliation instead of an
// automatic retry.
func IsUnknownOutcome(err error) bool {
	var unknown *UnknownOutcomeError
	return errors.As(err, &unknown)
}

// Dispatcher leases approved command outbox rows and records durable results.
type Dispatcher struct {
	db           *storage.DB
	effector     Effector
	clk          clock.Clock
	idGen        ids.Generator
	owner        string
	leaseFor     time.Duration
	runtimeOwner *storage.RuntimeOwner
	runtimeEpoch string
	interlock    interlock.Reader
	telemetry    *telemetry.Runtime
}

// WithRuntimeOwner fences dispatcher ledger mutations to the active runtime
// lease. It returns the dispatcher for composition during startup.
func (d *Dispatcher) WithRuntimeOwner(owner *storage.RuntimeOwner, epoch string) *Dispatcher {
	d.runtimeOwner = owner
	d.runtimeEpoch = epoch
	return d
}

// WithInterlock adds the final read-only readiness check before effect delivery.
func (d *Dispatcher) WithInterlock(reader interlock.Reader) *Dispatcher {
	d.interlock = reader
	return d
}

// WithTelemetry connects action-dispatch counters to the runtime telemetry
// surface. It is optional for embedders and tests.
func (d *Dispatcher) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *Dispatcher {
	if d != nil {
		d.telemetry = runtimeTelemetry
	}
	return d
}

// NewDispatcher creates an action dispatcher. leaseFor controls how long an
// abandoned lease remains protected from another dispatcher.
func NewDispatcher(db *storage.DB, effector Effector, clk clock.Clock, idGen ids.Generator, owner string, leaseFor time.Duration) *Dispatcher {
	if clk == nil {
		clk = clock.Physical()
	}
	if idGen == nil {
		idGen = ids.Random()
	}
	if owner == "" {
		owner = "actions"
	}
	if leaseFor <= 0 {
		leaseFor = time.Minute
	}
	return &Dispatcher{db: db, effector: effector, clk: clk, idGen: idGen, owner: owner, leaseFor: leaseFor}
}

type leasedCommand struct {
	OutboxID    int64
	Command     Command
	CommandJSON []byte
	CommandSHA  []byte
	LeaseOwner  string
	Traceparent string
	Tracestate  string
	Now         time.Time
}

// DispatchOnce processes at most one command. A leased command is marked
// dispatching before the provider call, and all ledger changes are committed
// atomically after the provider returns.
func (d *Dispatcher) DispatchOnce(ctx context.Context) (bool, error) {
	leased, found, err := d.lease(ctx)
	if err != nil || !found {
		return found, err
	}
	if d.effector == nil {
		return true, d.finalize(ctx, leased, Effect{}, errors.New("no effector configured"))
	}
	if err := d.revalidateAuthorization(ctx, leased); err != nil {
		return true, d.finalize(ctx, leased, Effect{}, fmt.Errorf("authorization revalidation failed: %w", err))
	}
	callTimeout := d.leaseFor - d.leaseFor/10
	if callTimeout <= 0 {
		callTimeout = d.leaseFor
	}
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	if d.interlock != nil {
		guarded, ok := d.effector.(AuthorizedEffector)
		if !ok {
			return true, d.finalize(ctx, leased, Effect{}, errors.New("configured effector does not enforce dispatch authorization"))
		}
		effect, dispatchErr := guarded.DispatchAuthorized(callCtx, leased.Command, Authorization{Check: func(checkCtx context.Context) error {
			return d.assertInterlock(checkCtx, leased.Command)
		}})
		if errors.Is(dispatchErr, context.DeadlineExceeded) {
			dispatchErr = &UnknownOutcomeError{Err: dispatchErr}
		}
		return true, d.finalizeDispatch(ctx, callCtx, leased, effect, dispatchErr)
	}
	effect, dispatchErr := d.effector.Dispatch(callCtx, leased.Command)
	if errors.Is(dispatchErr, context.DeadlineExceeded) {
		dispatchErr = &UnknownOutcomeError{Err: dispatchErr}
	}
	return true, d.finalizeDispatch(ctx, callCtx, leased, effect, dispatchErr)
}

func (d *Dispatcher) finalizeDispatch(ctx, verifyCtx context.Context, leased leasedCommand, effect Effect, dispatchErr error) error {
	verifier, canVerify := d.effector.(DeviceStateVerifier)
	finalStatus := ""
	var evidence map[string]any
	var verifyErr error
	if canVerify && (dispatchErr == nil || IsUnknownOutcome(dispatchErr)) {
		finalStatus, evidence, verifyErr = verifier.VerifyDeviceCommand(verifyCtx, leased.Command)
		if finalStatus != "" {
			effect.ObservedEffect = evidence
		}
		if dispatchErr == nil && verifyErr != nil {
			effect.VerificationPending = false
			dispatchErr = &UnknownOutcomeError{Err: fmt.Errorf("verify device state: %w", verifyErr)}
		} else if dispatchErr == nil && finalStatus != "" {
			effect.VerificationPending = false
			if finalStatus == "failed" {
				dispatchErr = errors.New("device state verification failed")
			}
		}
	}
	if err := d.finalize(ctx, leased, effect, dispatchErr); err != nil {
		return err
	}
	if IsUnknownOutcome(dispatchErr) && finalStatus != "" && verifyErr == nil {
		if err := d.ReconcileUnknown(ctx, leased.Command.CommandID, finalStatus, evidence); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) assertInterlock(ctx context.Context, command Command) error {
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := d.interlock.Assert(ctx, tx, command.TenantID, command.NormalizedTarget, ""); err != nil {
			return fmt.Errorf("dispatch interlock assertion: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("assert dispatch interlock: %w", err)
	}
	return nil
}

func (d *Dispatcher) revalidateAuthorization(ctx context.Context, leased leasedCommand) error {
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		return d.revalidateAuthorizationTx(ctx, tx, leased)
	}); err != nil {
		return fmt.Errorf("revalidate dispatch authorization: %w", err)
	}
	return nil
}

func (d *Dispatcher) revalidateAuthorizationTx(ctx context.Context, tx *sql.Tx, leased leasedCommand) error {
	if err := d.assertRuntimeOwner(ctx, tx); err != nil {
		return err
	}
	var leaseValid int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM outbox WHERE outbox_id = ? AND status = 'leased' AND lease_owner = ? AND lease_until > ?`, leased.OutboxID, leased.LeaseOwner, formatTime(d.clk.Now())).Scan(&leaseValid); err != nil {
		return fmt.Errorf("dispatch lease is no longer active: %w", err)
	}
	commandID := leased.Command.CommandID
	var commandTenant, commandIntent, commandRoute, commandTarget, intentTenant, intentID, decisionID string
	var intentSituation, intentType, intentRisk, intentExpires string
	var intentVersion, currentVersion, episodeVersion int
	var commandJSON, commandSHA, commandIdempotency, intentJSON, intentSHA, decisionJSON, decisionSHA []byte
	var approvedApprovalID, approvedApprovalExpiry sql.NullString
	var policyStatus, validationStatus, episodeID, episodeTenant, episodeSituation, situationTenant, episodeLifecycle, decisionSituation string
	var decisionVersion int
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
		&commandTenant, &commandIntent, &commandRoute, &commandTarget,
		&commandIdempotency, &commandJSON, &commandSHA,
		&intentTenant, &intentID, &decisionID, &intentSituation,
		&intentVersion, &intentType, &intentRisk, &intentJSON,
		&intentSHA, &intentExpires, &policyStatus,
		&approvedApprovalID, &approvedApprovalExpiry,
		&validationStatus, &decisionJSON, &decisionSHA, &decisionSituation, &decisionVersion, &episodeID,
		&episodeTenant, &episodeSituation, &episodeVersion, &episodeLifecycle,
		&situationTenant, &currentVersion); err != nil {
		return fmt.Errorf("load authorization records: %w", err)
	}
	var commandDocument map[string]any
	if err := json.Unmarshal(commandJSON, &commandDocument); err != nil || contractsv1.Validate(contractsv1.SchemaCommand, commandDocument) != nil || !verifyDigest(canonicaljson.DomainCommand, commandDocument, commandSHA) ||
		documentString(commandDocument, "command_id") != commandID || documentString(commandDocument, "intent_id") != commandIntent ||
		documentString(commandDocument, "tenant_id") != commandTenant || documentString(commandDocument, "effector_route") != commandRoute ||
		documentString(commandDocument, "normalized_target") != commandTarget || !bytes.Equal(commandIdempotency, mustDigest(commandDocument, "idempotency_key")) {
		return errors.New("command ledger identity mismatch")
	}
	if commandTenant != intentTenant || commandIntent != intentID || commandRoute != intentType || policyStatus != "approved" || validationStatus != "accepted" {
		return errors.New("command is no longer approved for its intent")
	}
	if d.interlock != nil {
		if err := d.interlock.Assert(ctx, tx, commandTenant, commandTarget, intentRisk); err != nil {
			return fmt.Errorf("interlock rejected command: %w", err)
		}
	}
	if intentRisk == "R2" {
		if !approvedApprovalID.Valid {
			return errors.New("approved intent has no approved approval record")
		}
		approvalExpires, parseErr := time.Parse(time.RFC3339Nano, approvedApprovalExpiry.String)
		if parseErr != nil || !approvalExpires.After(d.clk.Now()) {
			return errors.New("approval is expired")
		}
	}
	if episodeTenant != intentTenant || situationTenant != intentTenant || decisionSituation != intentSituation || decisionVersion != intentVersion || episodeSituation != intentSituation || episodeVersion != intentVersion || (episodeLifecycle != "concluded" && episodeLifecycle != "closed") || currentVersion != intentVersion {
		return errors.New("command authorization is stale")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, intentExpires)
	if err != nil || !expiresAt.After(d.clk.Now()) {
		return errors.New("intent authorization is expired")
	}
	var intentDocument map[string]any
	if err := json.Unmarshal(intentJSON, &intentDocument); err != nil || contractsv1.Validate(contractsv1.SchemaIntent, intentDocument) != nil || !verifyIntentDigest(intentDocument, intentSHA) {
		return errors.New("intent authorization is invalid")
	}
	if documentString(intentDocument, "intent_id") != intentID || documentString(intentDocument, "decision_id") != decisionID || documentString(intentDocument, "tenant_id") != intentTenant || documentString(intentDocument, "situation_id") != intentSituation || documentInt(intentDocument, "situation_version") != intentVersion || documentString(intentDocument, "type") != intentType || documentString(intentDocument, "risk_class") != intentRisk {
		return errors.New("intent authorization identity mismatch")
	}
	if commandPolicyDigest := documentString(commandDocument, "policy_digest"); commandPolicyDigest != "" {
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
	}
	var decisionDocument map[string]any
	if err := json.Unmarshal(decisionJSON, &decisionDocument); err != nil || contractsv1.Validate(contractsv1.SchemaDecision, decisionDocument) != nil || !verifyDigest(canonicaljson.DomainDecision, decisionDocument, decisionSHA) {
		return errors.New("decision authorization is invalid")
	}
	if documentString(decisionDocument, "decision_id") != decisionID || documentString(decisionDocument, "episode_id") != episodeID || documentString(decisionDocument, "situation_id") != intentSituation || documentInt(decisionDocument, "situation_version") != intentVersion {
		return errors.New("decision authorization identity mismatch")
	}
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

func (d *Dispatcher) lease(ctx context.Context) (leasedCommand, bool, error) {
	var leased leasedCommand
	found := false
	err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := d.assertRuntimeOwner(ctx, tx); err != nil {
			return err
		}
		now := d.clk.Now().UTC()
		var commandStatus string
		var storedIntentID, storedTenant, storedRoute, storedTarget string
		var storedIdempotency []byte
		var outboxStatus string
		var storedLeaseOwner, storedLeaseUntil sql.NullString
		var traceparent, tracestate sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT o.outbox_id, o.aggregate_id, c.command_json, c.command_sha256, c.status,
				c.intent_id, c.tenant_id, c.effector_route, c.normalized_target, c.idempotency_key,
				d.traceparent, d.tracestate
				, o.status, o.lease_owner, o.lease_until
			FROM outbox o
			JOIN commands c ON c.command_id = o.aggregate_id
			JOIN intents i ON i.intent_id = c.intent_id
			JOIN decisions d ON d.decision_id = i.decision_id
			WHERE o.kind = 'command'
			  AND o.available_at <= ?
			  AND (o.status = 'pending' OR (o.status = 'leased' AND o.lease_until <= ?))
			ORDER BY o.outbox_id
			LIMIT 1`, formatTime(now), formatTime(now)).Scan(
			&leased.OutboxID, &leased.Command.CommandID,
			&leased.CommandJSON, &leased.CommandSHA, &commandStatus,
			&storedIntentID, &storedTenant, &storedRoute, &storedTarget, &storedIdempotency,
			&traceparent, &tracestate,
			&outboxStatus, &storedLeaseOwner, &storedLeaseUntil,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("find command outbox: %w", err)
		}
		found = true
		leased.Now = now
		leased.Traceparent = traceparent.String
		leased.Tracestate = tracestate.String
		if outboxStatus == "leased" {
			leased.LeaseOwner = storedLeaseOwner.String
			if expiresAt, parseErr := time.Parse(time.RFC3339Nano, storedLeaseUntil.String); parseErr != nil || !storedLeaseOwner.Valid || !storedLeaseUntil.Valid || !expiresAt.After(now) {
				found = false
				if d.telemetry != nil {
					d.telemetry.ObserveLeaseExpiry()
				}
				return d.finalizeTx(ctx, tx, leased, Effect{}, &UnknownOutcomeError{Err: errors.New("lease expired before dispatch")})
			}
		}
		var document map[string]any
		if err := json.Unmarshal(leased.CommandJSON, &document); err != nil {
			found = false
			return d.markLeaseFailure(ctx, tx, leased.OutboxID, leased.Command.CommandID, "command_json_invalid", now)
		}
		if err := contractsv1.Validate(contractsv1.SchemaCommand, document); err != nil {
			found = false
			return d.markLeaseFailure(ctx, tx, leased.OutboxID, leased.Command.CommandID, "command_schema_invalid", now)
		}
		providedIdempotency := documentString(document, "idempotency_key")
		providedIdempotencyBytes, idempotencyErr := canonicaljson.DecodeDigest(providedIdempotency)
		if leased.Command.CommandID != documentString(document, "command_id") ||
			storedIntentID != documentString(document, "intent_id") || storedTenant != documentString(document, "tenant_id") ||
			storedRoute != documentString(document, "effector_route") || storedTarget != documentString(document, "normalized_target") ||
			idempotencyErr != nil || !bytes.Equal(storedIdempotency, providedIdempotencyBytes) ||
			!verifyDigest(canonicaljson.DomainCommand, document, leased.CommandSHA) {
			found = false
			return d.markLeaseFailure(ctx, tx, leased.OutboxID, leased.Command.CommandID, "command_digest_mismatch", now)
		}
		leased.Command.IntentID = documentString(document, "intent_id")
		leased.Command.TenantID = documentString(document, "tenant_id")
		leased.Command.EffectorRoute = documentString(document, "effector_route")
		leased.Command.NormalizedTarget = documentString(document, "normalized_target")
		leased.Command.IdempotencyKey = documentString(document, "idempotency_key")
		leased.Command.PolicyDigest = documentString(document, "policy_digest")
		leased.Command.NotBeforeMonoUS = documentInt64(document, "not_before_mono_us")
		leased.Command.Payload, _ = document["payload"].(map[string]any)
		if commandStatus == "succeeded" || commandStatus == "outcome_unknown" {
			found = false
			return d.finishOutboxOnly(ctx, tx, leased.OutboxID, commandStatus, now)
		}
		until := now.Add(d.leaseFor)
		leased.LeaseOwner = d.owner + "/" + d.idGen.New(ids.PrefixLease)
		result, err := tx.ExecContext(ctx, `
			UPDATE outbox
			SET status = 'leased', lease_owner = ?, lease_until = ?, attempt_count = attempt_count + 1
			WHERE outbox_id = ? AND (status = 'pending' OR (status = 'leased' AND lease_until <= ?))`,
			leased.LeaseOwner, formatTime(until), leased.OutboxID, formatTime(now))
		if err != nil {
			return fmt.Errorf("lease command outbox: %w", err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			found = false
			return nil
		}
		if _, err := tx.ExecContext(ctx, "UPDATE commands SET status = 'dispatching', updated_at = ? WHERE command_id = ? AND status IN ('pending', 'failed', 'dispatching')", formatTime(now), leased.Command.CommandID); err != nil {
			return fmt.Errorf("mark command dispatching: %w", err)
		}
		return nil
	})
	if err != nil {
		return leasedCommand{}, false, fmt.Errorf("lease command: %w", err)
	}
	return leased, found, nil
}

func (d *Dispatcher) finalize(ctx context.Context, leased leasedCommand, effect Effect, dispatchErr error) error {
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		return d.finalizeTx(ctx, tx, leased, effect, dispatchErr)
	}); err != nil {
		return fmt.Errorf("finalize command dispatch: %w", err)
	}
	return nil
}

func (d *Dispatcher) finalizeTx(ctx context.Context, tx *sql.Tx, leased leasedCommand, effect Effect, dispatchErr error) error {
	if err := d.assertRuntimeOwner(ctx, tx); err != nil {
		return err
	}
	now := d.clk.Now().UTC()
	var leaseStatus, leaseOwner, leaseUntil string
	if err := tx.QueryRowContext(ctx, `
			SELECT status, lease_owner, lease_until FROM outbox WHERE outbox_id = ?`,
		leased.OutboxID).Scan(&leaseStatus, &leaseOwner, &leaseUntil); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Another dispatcher owns the lease, or this worker's lease
			// expired. A late provider result must not overwrite it.
			return nil
		}
		return fmt.Errorf("verify command lease: %w", err)
	}
	if leaseStatus != "leased" || leaseOwner != leased.LeaseOwner {
		return nil
	}
	if expiresAt, err := time.Parse(time.RFC3339Nano, leaseUntil); err != nil || !expiresAt.After(now) {
		// The provider result arrived after the lease window. Treat it as
		// unknown so the next worker cannot blindly repeat the effect.
		dispatchErr = &UnknownOutcomeError{Err: errors.New("lease expired before provider result")}
		effect = Effect{}
		if d.telemetry != nil {
			d.telemetry.ObserveLeaseExpiry()
		}
	}
	status := "succeeded"
	reconciliation := "observed"
	commandStatus := "succeeded"
	errorCode := ""
	if dispatchErr != nil {
		if IsUnknownOutcome(dispatchErr) {
			status, reconciliation, commandStatus = "unknown", "required", "reconciling"
			errorCode = "outcome_unknown"
		} else {
			status, reconciliation, commandStatus = "failed", "not_required", "failed"
			errorCode = "dispatch_failed"
		}
	}
	if dispatchErr == nil && effect.VerificationPending {
		// A transport receipt is not physical success. Keep the command in the
		// existing non-terminal manual-review state until an independent
		// feedback verifier closes it; the outbox is delivered because no blind
		// resend is safe after the provider accepted the frame.
		status = "reconcile_required"
		reconciliation = "required"
		commandStatus = "manual_review"
	}
	outcomeID := d.idGen.New(ids.PrefixOutcome)
	document := map[string]any{
		"outcome_id": outcomeID, "command_id": leased.Command.CommandID,
		"status": status, "observed_at": formatTime(now),
	}
	if effect.ProviderResult != nil {
		document["result"] = effect.ProviderResult
	}
	if errorCode != "" {
		document["error_code"] = errorCode
	}
	if err := contractsv1.Validate(contractsv1.SchemaOutcome, document); err != nil {
		return fmt.Errorf("validate action outcome: %w", err)
	}
	outcomeDigest, err := canonicaljson.Digest(canonicaljson.DomainOutcome, document)
	if err != nil {
		return fmt.Errorf("digest action outcome: %w", err)
	}
	outcomeSHA, err := canonicaljson.DecodeDigest(outcomeDigest)
	if err != nil {
		return fmt.Errorf("decode action outcome digest: %w", err)
	}
	providerJSON, err := optionalJSON(effect.ProviderResult)
	if err != nil {
		return fmt.Errorf("encode provider result: %w", err)
	}
	observedJSON, err := optionalJSON(effect.ObservedEffect)
	if err != nil {
		return fmt.Errorf("encode observed effect: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
			INSERT INTO outcomes (
				outcome_id, command_id, ordinal, status, provider_result_json,
				observed_effect_json, reconciliation_status, outcome_sha256, traceparent, tracestate, occurred_at
			) VALUES (?, ?, (SELECT COALESCE(MAX(ordinal), 0) + 1 FROM outcomes WHERE command_id = ?), ?, ?, ?, ?, ?, ?, ?, ?)`,
		outcomeID, leased.Command.CommandID, leased.Command.CommandID, status, providerJSON,
		observedJSON, reconciliation, outcomeSHA, nullableString(leased.Traceparent), nullableString(leased.Tracestate), formatTime(now)); err != nil {
		return fmt.Errorf("record action outcome: %w", err)
	}
	outboxStatus := "delivered"
	if dispatchErr != nil {
		outboxStatus = "failed"
	}
	if _, err := tx.ExecContext(ctx, `
			UPDATE commands SET status = ?, updated_at = ? WHERE command_id = ? AND status = 'dispatching'`,
		commandStatus, formatTime(now), leased.Command.CommandID); err != nil {
		return fmt.Errorf("record command status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
			UPDATE outbox SET status = ?, lease_owner = NULL, lease_until = NULL,
				last_error_code = ?, delivered_at = CASE WHEN ? = 'delivered' THEN ? ELSE delivered_at END
			WHERE outbox_id = ? AND lease_owner = ?`,
		outboxStatus, nullableString(errorCode), outboxStatus, formatTime(now), leased.OutboxID, leased.LeaseOwner); err != nil {
		return fmt.Errorf("finish command outbox: %w", err)
	}
	verificationStatus := "observed"
	if dispatchErr != nil || effect.VerificationPending {
		verificationStatus = "awaiting"
	}
	if _, err := tx.ExecContext(ctx, `
			INSERT INTO verifications (verification_id, intent_id, command_id, outcome_id, status, updated_at)
			SELECT ?, intent_id, command_id, ?, ?, ? FROM commands WHERE command_id = ?
			ON CONFLICT(intent_id) DO UPDATE SET outcome_id = excluded.outcome_id,
				status = excluded.status, updated_at = excluded.updated_at`,
		d.idGen.New(ids.PrefixVerification), outcomeID, verificationStatus, formatTime(now), leased.Command.CommandID); err != nil {
		return fmt.Errorf("record command verification: %w", err)
	}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "command.dispatched:"+leased.Command.CommandID+":"+outcomeID, leased.Command.TenantID, notify.TypeCommandDispatched, "command/"+leased.Command.CommandID, leased.Command.CommandID, map[string]any{
		"tenant_id": leased.Command.TenantID, "command_id": leased.Command.CommandID, "intent_id": leased.Command.IntentID,
		"outcome_id": outcomeID, "status": commandStatus, "source_authority": notify.SourceForTenant(leased.Command.TenantID),
	}, now, contractsv1.TraceContext{Traceparent: leased.Traceparent, Tracestate: leased.Tracestate}); err != nil {
		return fmt.Errorf("append command dispatched notification: %w", err)
	}
	notificationStatus := status
	if notificationStatus == "reconcile_required" {
		// The notification contract deliberately calls an unverified physical
		// result unknown, while the durable outcome keeps the more precise
		// reconcile_required state for internal consumers.
		notificationStatus = "unknown"
	}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "outcome.recorded:"+outcomeID, leased.Command.TenantID, notify.TypeOutcomeRecorded, "outcome/"+outcomeID, leased.Command.CommandID, map[string]any{
		"tenant_id": leased.Command.TenantID, "outcome_id": outcomeID, "command_id": leased.Command.CommandID, "status": notificationStatus,
		"reconciliation_status": reconciliation, "intent_id": leased.Command.IntentID,
		"outcome_digest":   "sha256:" + hex.EncodeToString(outcomeSHA),
		"source_authority": notify.SourceForTenant(leased.Command.TenantID),
	}, now, contractsv1.TraceContext{Traceparent: leased.Traceparent, Tracestate: leased.Tracestate}); err != nil {
		return fmt.Errorf("append outcome recorded notification: %w", err)
	}
	if !effect.VerificationPending && (status == "succeeded" || status == "failed") {
		if err := appendOutcomeReconciledNotification(ctx, tx, leased.Command.TenantID, leased.Command.IntentID, leased.Command.CommandID, outcomeID, status, now); err != nil {
			return fmt.Errorf("append outcome reconciled notification: %w", err)
		}
	}
	return nil
}

// ReconcileUnknown closes an outcome_unknown command using independently
// observed provider evidence. It is the only path that may resolve a command
// after an effector call whose result was uncertain.
func (d *Dispatcher) ReconcileUnknown(ctx context.Context, commandID, finalStatus string, evidence map[string]any) error {
	if finalStatus != "succeeded" && finalStatus != "failed" && finalStatus != "manual_review" {
		return fmt.Errorf("invalid reconciliation status %q", finalStatus)
	}
	if err := validateReconciliationEvidence(evidence); err != nil {
		return err
	}
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := d.assertRuntimeOwner(ctx, tx); err != nil {
			return err
		}
		var currentStatus, tenantID, intentID, commandTarget string
		var traceparent, tracestate sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT c.status, c.tenant_id, c.intent_id, c.normalized_target, d.traceparent, d.tracestate
			FROM commands c
			JOIN intents i ON i.intent_id = c.intent_id
			JOIN decisions d ON d.decision_id = i.decision_id
			WHERE c.command_id = ?`, commandID).Scan(&currentStatus, &tenantID, &intentID, &commandTarget, &traceparent, &tracestate); err != nil {
			return fmt.Errorf("load command %s for reconciliation: %w", commandID, err)
		}
		if currentStatus != "reconciling" && currentStatus != "outcome_unknown" && currentStatus != "manual_review" {
			return fmt.Errorf("command %s is not awaiting reconciliation", commandID)
		}
		var boundTarget, deviceID, bootID string
		bindingErr := tx.QueryRowContext(ctx, `SELECT target, device_id, boot_id
			FROM device_command_bindings WHERE command_id = ?`, commandID).Scan(&boundTarget, &deviceID, &bootID)
		switch {
		case bindingErr == nil:
			if boundTarget != commandTarget {
				return fmt.Errorf("device command binding target does not match command %q", commandID)
			}
			if evidenceTarget, _ := evidence["target"].(string); evidenceTarget != boundTarget {
				return fmt.Errorf("device reconciliation evidence target does not match command %q", commandID)
			}
			if err := storage.ValidateDeviceReconciliationEvidence(evidence, deviceID, bootID); err != nil {
				return fmt.Errorf("validate device reconciliation evidence: %w", err)
			}
		case !errors.Is(bindingErr, sql.ErrNoRows):
			return fmt.Errorf("load device command binding: %w", bindingErr)
		}
		now := d.clk.Now().UTC()
		outcomeID := d.idGen.New(ids.PrefixOutcome)
		document := map[string]any{
			"outcome_id": outcomeID, "command_id": commandID, "status": "reconciled",
			"observed_at": formatTime(now),
			"result":      map[string]any{"final_status": finalStatus, "evidence": evidence},
		}
		if err := contractsv1.Validate(contractsv1.SchemaOutcome, document); err != nil {
			return fmt.Errorf("validate reconciliation outcome: %w", err)
		}
		digest, err := canonicaljson.Digest(canonicaljson.DomainOutcome, document)
		if err != nil {
			return fmt.Errorf("digest reconciliation outcome: %w", err)
		}
		outcomeSHA, err := canonicaljson.DecodeDigest(digest)
		if err != nil {
			return fmt.Errorf("decode reconciliation digest: %w", err)
		}
		evidenceJSON, err := optionalJSON(evidence)
		if err != nil {
			return fmt.Errorf("encode reconciliation evidence: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO outcomes (
				outcome_id, command_id, ordinal, status, provider_result_json,
				observed_effect_json, reconciliation_status, outcome_sha256, traceparent, tracestate, occurred_at
			) VALUES (?, ?, (SELECT COALESCE(MAX(ordinal), 0) + 1 FROM outcomes WHERE command_id = ?), 'reconciled', ?, NULL, 'reconciled', ?, ?, ?, ?)`,
			outcomeID, commandID, commandID, evidenceJSON, outcomeSHA, traceparent, tracestate, formatTime(now)); err != nil {
			return fmt.Errorf("record reconciliation outcome: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE commands SET status = ?, updated_at = ? WHERE command_id = ? AND status IN ('reconciling', 'outcome_unknown')", finalStatus, formatTime(now), commandID); err != nil {
			return fmt.Errorf("close reconciled command: %w", err)
		}
		verificationStatus := "inconclusive"
		switch finalStatus {
		case "succeeded":
			verificationStatus = "reconciled"
		case "failed":
			verificationStatus = "refuted"
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE verifications SET outcome_id = ?, status = ?, reconciled_at = ?, updated_at = ?
			WHERE command_id = ?`, outcomeID, verificationStatus, formatTime(now), formatTime(now), commandID); err != nil {
			return fmt.Errorf("update reconciliation verification: %w", err)
		}
		if err := appendOutcomeReconciledNotification(ctx, tx, tenantID, intentID, commandID, outcomeID, finalStatus, now); err != nil {
			return fmt.Errorf("append outcome reconciled notification: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile unknown command: %w", err)
	}
	return nil
}

func validateReconciliationEvidence(evidence map[string]any) error {
	if len(evidence) == 0 {
		return fmt.Errorf("reconciliation evidence is required")
	}
	source, _ := evidence["source"].(string)
	if source == "" {
		return fmt.Errorf("reconciliation evidence source is required")
	}
	evidenceType, _ := evidence["evidence_type"].(string)
	if evidenceType != "provider_observation" && evidenceType != "device_state_feedback" {
		return fmt.Errorf("reconciliation evidence_type is required")
	}
	if evidenceType == "device_state_feedback" {
		for _, key := range []string{"device_id", "boot_id", "state", "feedback_digest"} {
			if _, ok := evidence[key]; !ok {
				return fmt.Errorf("device reconciliation evidence requires %s", key)
			}
		}
	}
	for _, key := range []string{"evidence_digest", "state_digest", "feedback_digest"} {
		if digest, ok := evidence[key].(string); ok && digest != "" {
			if _, err := canonicaljson.DecodeDigest(digest); err != nil {
				return fmt.Errorf("invalid reconciliation %s: %w", key, err)
			}
			return nil
		}
	}
	return fmt.Errorf("reconciliation evidence must include a sha256 evidence, state, or feedback digest")
}

func appendOutcomeReconciledNotification(ctx context.Context, tx *sql.Tx, tenantID, intentID, commandID, outcomeID, finalStatus string, now time.Time) error {
	var reconciliationVersion int
	var outcomeSHA []byte
	var traceparent, tracestate sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT o.ordinal, o.outcome_sha256, d.traceparent, d.tracestate
		FROM outcomes o
		JOIN commands c ON c.command_id = o.command_id
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE o.outcome_id = ? AND o.command_id = ? AND i.intent_id = ?`, outcomeID, commandID, intentID).
		Scan(&reconciliationVersion, &outcomeSHA, &traceparent, &tracestate); err != nil {
		return fmt.Errorf("load reconciled outcome provenance: %w", err)
	}
	if reconciliationVersion < 1 || len(outcomeSHA) != sha256.Size {
		return fmt.Errorf("reconciled outcome provenance is incomplete")
	}
	return notify.AppendLifecycleEventWithTrace(ctx, tx, "outcome.reconciled:"+outcomeID, tenantID, notify.TypeOutcomeReconciled, "outcome/"+outcomeID, commandID, map[string]any{
		"tenant_id": tenantID, "outcome_id": outcomeID, "command_id": commandID, "final_status": finalStatus,
		"outcome_digest": "sha256:" + hex.EncodeToString(outcomeSHA), "reconciliation_status": "reconciled", "intent_id": intentID,
		"verdict": outcomeVerdict(finalStatus), "reconciliation_version": reconciliationVersion,
		"source_authority": notify.SourceForTenant(tenantID),
	}, now, contractsv1.TraceContext{Traceparent: traceparent.String, Tracestate: tracestate.String})
}

func outcomeVerdict(finalStatus string) string {
	switch finalStatus {
	case "succeeded":
		return "verified"
	case "failed":
		return "refuted"
	default:
		return "inconclusive"
	}
}

func (d *Dispatcher) assertRuntimeOwner(ctx context.Context, tx *sql.Tx) error {
	if d.runtimeOwner == nil || d.runtimeEpoch == "" {
		return nil
	}
	if err := d.runtimeOwner.Assert(ctx, tx, d.runtimeEpoch); err != nil {
		return fmt.Errorf("action runtime ownership lost: %w", err)
	}
	return nil
}

func (d *Dispatcher) markLeaseFailure(ctx context.Context, tx *sql.Tx, outboxID int64, commandID, code string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, "UPDATE commands SET status = 'failed', updated_at = ? WHERE command_id = ?", formatTime(now), commandID); err != nil {
		return fmt.Errorf("mark invalid command failed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE outbox SET status = 'failed', last_error_code = ?, lease_owner = NULL, lease_until = NULL WHERE outbox_id = ?`, code, outboxID); err != nil {
		return fmt.Errorf("mark invalid outbox failed: %w", err)
	}
	return nil
}

func (d *Dispatcher) finishOutboxOnly(ctx context.Context, tx *sql.Tx, outboxID int64, commandStatus string, now time.Time) error {
	status := "delivered"
	if commandStatus == "outcome_unknown" {
		status = "failed"
	}
	_, err := tx.ExecContext(ctx, `UPDATE outbox SET status = ?, lease_owner = NULL, lease_until = NULL, delivered_at = CASE WHEN ? = 'delivered' THEN ? ELSE delivered_at END WHERE outbox_id = ?`, status, status, formatTime(now), outboxID)
	if err != nil {
		return fmt.Errorf("finish outbox: %w", err)
	}
	return nil
}

func verifyDigest(domain canonicaljson.Domain, document map[string]any, digest []byte) bool {
	if len(digest) != sha256.Size {
		return false
	}
	return canonicaljson.Verify(domain, document, "sha256:"+hex.EncodeToString(digest))
}

func verifyIntentDigest(document map[string]any, digest []byte) bool {
	if len(digest) != sha256.Size || !contractsv1.VerifyIntentDigest(document) {
		return false
	}
	expected, err := contractsv1.IntentDigest(document)
	if err != nil {
		return false
	}
	return expected == "sha256:"+hex.EncodeToString(digest)
}

func documentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func documentInt(document map[string]any, key string) int {
	value, _ := document[key].(float64)
	return int(value)
}

func documentInt64(document map[string]any, key string) int64 {
	value, _ := document[key].(float64)
	return int64(value)
}

func mustDigest(document map[string]any, key string) []byte {
	value, _ := document[key].(string)
	digest, err := canonicaljson.DecodeDigest(value)
	if err != nil {
		return nil
	}
	return digest
}

func optionalJSON(value map[string]any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := canonicaljson.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode optional JSON: %w", err)
	}
	return encoded, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
