// Package policy owns the deterministic authorization boundary between
// accepted Intents and Commands.
package policy

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Result is the durable policy result for one Intent evaluation.
type Result struct {
	IntentID   string
	DecisionID string
	Result     string
	Reason     string
	CommandID  string
	ApprovalID string
}

// Gateway evaluates accepted Intents against current durable state.
type Gateway struct {
	policyVersion string
	policyDigest  string
	idGen         ids.Generator
	owner         *storage.RuntimeOwner
	ownerEpoch    string
}

// NewGateway creates a deterministic policy gateway.
func NewGateway(policyVersion string, idGen ids.Generator) *Gateway {
	return newGateway(policyVersion, idGen, nil, "")
}

// NewGatewayWithOwner creates a policy gateway that fences every mutation to
// the active runtime lease.
func NewGatewayWithOwner(policyVersion string, idGen ids.Generator, owner *storage.RuntimeOwner, ownerEpoch string) *Gateway {
	return newGateway(policyVersion, idGen, owner, ownerEpoch)
}

func newGateway(policyVersion string, idGen ids.Generator, owner *storage.RuntimeOwner, ownerEpoch string) *Gateway {
	if idGen == nil {
		idGen = ids.Random()
	}
	policyDigest, _ := canonicaljson.Digest(canonicaljson.DomainTest, map[string]any{"policy_version": policyVersion})
	return &Gateway{policyVersion: policyVersion, policyDigest: policyDigest, idGen: idGen, owner: owner, ownerEpoch: ownerEpoch}
}

type intentRow struct {
	IntentID          string
	DecisionID        string
	EpisodeID         string
	EpisodeTenant     string
	EpisodeSituation  string
	EpisodeVersion    int
	SituationTenant   string
	DecisionSituation string
	DecisionVersion   int
	TenantID          string
	SituationID       string
	SituationVersion  int
	IntentType        string
	RiskClass         string
	IntentJSON        []byte
	IntentSHA         []byte
	ExpiresAt         string
	PolicyStatus      string
	ValidationStatus  string
	DecisionJSON      []byte
	DecisionSHA       []byte
	EpisodeLifecycle  string
	CurrentSituation  int
}

// EvaluateIntent runs the full v1 policy order and atomically creates either
// an approval request or a Command plus outbox row. Repeated evaluation is
// idempotent for the effect-producing branches.
func (g *Gateway) EvaluateIntent(ctx context.Context, tx *sql.Tx, intentID string, now time.Time) (Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return Result{IntentID: intentID}, err
	}
	row, err := g.loadIntent(ctx, tx, intentID)
	if err != nil {
		return Result{IntentID: intentID}, err
	}
	result := Result{IntentID: row.IntentID, DecisionID: row.DecisionID}
	if row.PolicyStatus != "pending" {
		if row.PolicyStatus == "approval_required" {
			var approvalStatus, approvalExpiry string
			if err := tx.QueryRowContext(ctx, "SELECT status, expires_at FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&approvalStatus, &approvalExpiry); err == nil {
				if expiresAt, parseErr := time.Parse(time.RFC3339Nano, approvalExpiry); parseErr != nil || !expiresAt.After(now) {
					if _, err := tx.ExecContext(ctx, "UPDATE approvals SET status = 'expired' WHERE intent_id = ? AND status = 'pending'", row.IntentID); err != nil {
						return result, fmt.Errorf("expire approval: %w", err)
					}
					return g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
				}
			}
		}
		result.Result = row.PolicyStatus
		result.Reason = "already_evaluated"
		if row.PolicyStatus == "approved" {
			_ = tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", row.IntentID).Scan(&result.CommandID)
		}
		if row.PolicyStatus == "approval_required" {
			_ = tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&result.ApprovalID)
		}
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
	}

	if row.ValidationStatus != "accepted" {
		return g.finish(ctx, tx, row, result, "denied", "decision_not_accepted", now)
	}
	var decisionDocument map[string]any
	if err := json.Unmarshal(row.DecisionJSON, &decisionDocument); err != nil {
		return g.finish(ctx, tx, row, result, "denied", "schema_invalid", now)
	}
	if err := contractsv1.Validate(contractsv1.SchemaDecision, decisionDocument); err != nil {
		return g.finish(ctx, tx, row, result, "denied", "schema_invalid", now)
	}
	if documentString(decisionDocument, "decision_id") != row.DecisionID ||
		documentString(decisionDocument, "episode_id") != row.EpisodeID ||
		documentString(decisionDocument, "situation_id") != row.SituationID ||
		documentInt(decisionDocument, "situation_version") != row.SituationVersion ||
		row.EpisodeTenant != row.TenantID || row.SituationTenant != row.TenantID ||
		row.DecisionSituation != row.SituationID || row.DecisionVersion != row.SituationVersion ||
		row.EpisodeSituation != row.SituationID || row.EpisodeVersion != row.SituationVersion {
		return g.finish(ctx, tx, row, result, "denied", "identity_mismatch", now)
	}
	if !canonicalDocumentMatches(row.DecisionJSON, row.DecisionSHA, canonicaljson.DomainDecision) {
		return g.finish(ctx, tx, row, result, "denied", "decision_digest_mismatch", now)
	}
	var intentDocument map[string]any
	if err := json.Unmarshal(row.IntentJSON, &intentDocument); err != nil {
		return g.finish(ctx, tx, row, result, "denied", "schema_invalid", now)
	}
	if err := contractsv1.Validate(contractsv1.SchemaIntent, intentDocument); err != nil {
		return g.finish(ctx, tx, row, result, "denied", "schema_invalid", now)
	}
	if !canonicalDocumentMatches(row.IntentJSON, row.IntentSHA, canonicaljson.DomainIntent) {
		return g.finish(ctx, tx, row, result, "denied", "intent_digest_mismatch", now)
	}
	if documentString(intentDocument, "intent_id") != row.IntentID ||
		documentString(intentDocument, "decision_id") != row.DecisionID ||
		documentString(intentDocument, "tenant_id") != row.TenantID ||
		documentString(intentDocument, "situation_id") != row.SituationID ||
		documentInt(intentDocument, "situation_version") != row.SituationVersion ||
		documentString(intentDocument, "type") != row.IntentType ||
		documentString(intentDocument, "risk_class") != row.RiskClass {
		return g.finish(ctx, tx, row, result, "denied", "identity_mismatch", now)
	}
	if row.EpisodeLifecycle != "concluded" && row.EpisodeLifecycle != "closed" {
		return g.finish(ctx, tx, row, result, "denied", "episode_not_concluded", now)
	}
	if row.CurrentSituation != row.SituationVersion {
		if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'stale', updated_at = ? WHERE intent_id = ?", formatTime(now), row.IntentID); err != nil {
			return result, fmt.Errorf("mark stale intent: %w", err)
		}
		return g.audit(ctx, tx, row, result, "stale", "situation_version_stale", now)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expiresAt.After(now) {
		return g.finish(ctx, tx, row, result, "expired", "intent_expired", now)
	}

	switch row.RiskClass {
	case "R0", "R1":
		return g.approveAutomatic(ctx, tx, row, intentDocument, result, now)
	case "R2":
		var approvedApproval string
		if err := tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'approved' ORDER BY decided_at DESC LIMIT 1", row.IntentID).Scan(&approvedApproval); err == nil {
			result.ApprovalID = approvedApproval
			result.Reason = "approved_by_human"
			return g.approveAutomatic(ctx, tx, row, intentDocument, result, now)
		}
		return g.requireApproval(ctx, tx, row, intentDocument, result, expiresAt, now)
	case "R3", "R4":
		return g.finish(ctx, tx, row, result, "denied", "risk_policy_denied", now)
	default:
		return g.finish(ctx, tx, row, result, "denied", "unknown_risk_class", now)
	}
}

// ResolveApproval records a human approval decision and, when approved,
// immediately re-runs the full policy gate before creating a Command.
func (g *Gateway) ResolveApproval(ctx context.Context, tx *sql.Tx, approvalID string, approved bool, approver, reason string, now time.Time) (Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return Result{ApprovalID: approvalID}, err
	}
	var intentID, status, expiresAt string
	if err := tx.QueryRowContext(ctx, "SELECT intent_id, status, expires_at FROM approvals WHERE approval_id = ?", approvalID).Scan(&intentID, &status, &expiresAt); err != nil {
		return Result{ApprovalID: approvalID}, fmt.Errorf("load approval %s: %w", approvalID, err)
	}
	row, err := g.loadIntent(ctx, tx, intentID)
	if err != nil {
		return Result{IntentID: intentID, ApprovalID: approvalID}, err
	}
	result := Result{IntentID: intentID, DecisionID: row.DecisionID, ApprovalID: approvalID}
	if status != "pending" {
		result.Result = status
		result.Reason = "approval_already_resolved"
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
	}
	expires, parseErr := time.Parse(time.RFC3339Nano, expiresAt)
	if parseErr != nil || !expires.After(now) {
		if _, err := tx.ExecContext(ctx, "UPDATE approvals SET status = 'expired', decided_at = ?, reason = ? WHERE approval_id = ? AND status = 'pending'", formatTime(now), "approval_expired", approvalID); err != nil {
			return result, fmt.Errorf("expire approval %s: %w", approvalID, err)
		}
		return g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
	}
	approvalStatus := "denied"
	policyStatus := "denied"
	if approved {
		approvalStatus = "approved"
		policyStatus = "pending"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE approvals SET status = ?, decided_at = ?, approver_identity = ?, reason = ?
		WHERE approval_id = ? AND status = 'pending'`, approvalStatus, formatTime(now), approver, reason, approvalID); err != nil {
		return result, fmt.Errorf("resolve approval %s: %w", approvalID, err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = ?, updated_at = ? WHERE intent_id = ?", policyStatus, formatTime(now), intentID); err != nil {
		return result, fmt.Errorf("update approved intent %s: %w", intentID, err)
	}
	if !approved {
		return g.audit(ctx, tx, row, result, "denied", "approval_denied", now)
	}
	return g.EvaluateIntent(ctx, tx, intentID, now)
}

func (g *Gateway) assertOwner(ctx context.Context, tx *sql.Tx) error {
	if g.owner == nil || g.ownerEpoch == "" {
		return nil
	}
	if err := g.owner.Assert(ctx, tx, g.ownerEpoch); err != nil {
		return fmt.Errorf("policy runtime ownership lost: %w", err)
	}
	return nil
}

func (g *Gateway) loadIntent(ctx context.Context, tx *sql.Tx, intentID string) (intentRow, error) {
	var row intentRow
	err := tx.QueryRowContext(ctx, `
		SELECT i.intent_id, i.decision_id, i.tenant_id, i.situation_id,
		       i.situation_version, i.intent_type, i.risk_class, i.intent_json,
		       i.intent_sha256, i.expires_at, i.policy_status,
		       d.validation_status, d.raw_json, d.decision_sha256,
		       d.situation_id, d.situation_version,
			       e.episode_id, e.tenant_id, e.situation_id, e.situation_version,
			       e.lifecycle_status, s.tenant_id, s.current_version
		FROM intents i
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN episodes e ON e.episode_id = d.episode_id
		JOIN situations s ON s.situation_id = i.situation_id
		WHERE i.intent_id = ?`, intentID,
	).Scan(
		&row.IntentID, &row.DecisionID, &row.TenantID, &row.SituationID,
		&row.SituationVersion, &row.IntentType, &row.RiskClass, &row.IntentJSON,
		&row.IntentSHA, &row.ExpiresAt, &row.PolicyStatus,
		&row.ValidationStatus, &row.DecisionJSON, &row.DecisionSHA,
		&row.DecisionSituation, &row.DecisionVersion,
		&row.EpisodeID, &row.EpisodeTenant, &row.EpisodeSituation, &row.EpisodeVersion,
		&row.EpisodeLifecycle, &row.SituationTenant, &row.CurrentSituation,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return row, fmt.Errorf("intent %s not found", intentID)
	}
	if err != nil {
		return row, fmt.Errorf("load intent %s: %w", intentID, err)
	}
	return row, nil
}

func (g *Gateway) approveAutomatic(ctx context.Context, tx *sql.Tx, row intentRow, intentDocument map[string]any, result Result, now time.Time) (Result, error) {
	var existingCommand string
	if err := tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", row.IntentID).Scan(&existingCommand); err == nil {
		result.Result = "approved"
		result.Reason = "already_commanded"
		result.CommandID = existingCommand
		return g.audit(ctx, tx, row, result, "approved", result.Reason, now)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, fmt.Errorf("find existing command: %w", err)
	}

	commandID := g.idGen.New(ids.PrefixCommand)
	target := normalizedTarget(row.IntentID, intentDocument)
	idempotency := sha256.Sum256([]byte(row.TenantID + "|" + row.IntentID + "|" + row.IntentType + "|" + target))
	commandDocument := map[string]any{
		"command_id":        commandID,
		"intent_id":         row.IntentID,
		"tenant_id":         row.TenantID,
		"effector_route":    row.IntentType,
		"normalized_target": target,
		"idempotency_key":   "sha256:" + hex.EncodeToString(idempotency[:]),
		"status":            "prepared",
		"payload":           intentDocument["parameters"],
		"created_at":        formatTime(now),
	}
	commandJSON, err := canonicaljson.Marshal(commandDocument)
	if err != nil {
		return result, fmt.Errorf("canonicalize command: %w", err)
	}
	commandDigest, err := canonicaljson.Digest(canonicaljson.DomainCommand, commandDocument)
	if err != nil {
		return result, fmt.Errorf("digest command: %w", err)
	}
	commandSHA, err := canonicaljson.DecodeDigest(commandDigest)
	if err != nil {
		return result, fmt.Errorf("decode command digest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO commands (
			command_id, intent_id, tenant_id, effector_route, normalized_target,
			idempotency_key, command_json, command_sha256, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(intent_id) DO NOTHING`,
		commandID, row.IntentID, row.TenantID, row.IntentType, target,
		idempotency[:], commandJSON, commandSHA, formatTime(now), formatTime(now),
	); err != nil {
		return result, fmt.Errorf("insert command: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", row.IntentID).Scan(&commandID); err != nil {
		return result, fmt.Errorf("read command identity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox (
			kind, aggregate_id, aggregate_version, payload_json, status,
			available_at, created_at
		) VALUES ('command', ?, 1, ?, 'pending', ?, ?)
		ON CONFLICT(kind, aggregate_id, aggregate_version) DO NOTHING`,
		commandID, commandJSON, formatTime(now), formatTime(now),
	); err != nil {
		return result, fmt.Errorf("insert command outbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'approved', updated_at = ? WHERE intent_id = ?", formatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("approve intent: %w", err)
	}
	if result.Reason == "" {
		result.Reason = "automatic_r0_r1"
	}
	result.Result, result.CommandID = "approved", commandID
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func (g *Gateway) requireApproval(ctx context.Context, tx *sql.Tx, row intentRow, intentDocument map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	var approvalID string
	if err := tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&approvalID); err == nil {
		result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", approvalID
		return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, fmt.Errorf("find pending approval: %w", err)
	}
	approvalID = g.idGen.New(ids.PrefixApproval)
	approvalJSON, err := canonicaljson.Marshal(map[string]any{
		"approval_id": approvalID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"tenant_id": row.TenantID, "situation_id": row.SituationID,
		"situation_version": row.SituationVersion, "risk_class": row.RiskClass,
		"intent": intentDocument,
	})
	if err != nil {
		return result, fmt.Errorf("canonicalize approval: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approvals (
			approval_id, intent_id, status, requested_at, expires_at, approval_json
		) VALUES (?, ?, 'pending', ?, ?, ?)`,
		approvalID, row.IntentID, formatTime(now), formatTime(expiresAt), approvalJSON,
	); err != nil {
		return result, fmt.Errorf("insert approval: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'approval_required', updated_at = ? WHERE intent_id = ?", formatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("mark approval required: %w", err)
	}
	result.Result, result.Reason, result.ApprovalID = "approval_required", "risk_requires_approval", approvalID
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func (g *Gateway) finish(ctx context.Context, tx *sql.Tx, row intentRow, result Result, policyStatus, reason string, now time.Time) (Result, error) {
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = ?, updated_at = ? WHERE intent_id = ?", policyStatus, formatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("set intent policy status: %w", err)
	}
	return g.audit(ctx, tx, row, result, policyStatus, reason, now)
}

func (g *Gateway) audit(ctx context.Context, tx *sql.Tx, row intentRow, result Result, policyResult, reason string, now time.Time) (Result, error) {
	result.Result, result.Reason = policyResult, reason
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO policy_evaluations (
			evaluation_id, intent_id, decision_id, policy_version, result,
			policy_digest, intent_sha256, decision_sha256, command_id, approval_id,
			reason, situation_version, evaluated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.idGen.New(ids.PrefixPolicy), row.IntentID, row.DecisionID, g.policyVersion,
		policyResult, g.policyDigest, row.IntentSHA, row.DecisionSHA,
		nullableID(result.CommandID), nullableID(result.ApprovalID), reason, row.CurrentSituation, formatTime(now),
	); err != nil {
		return result, fmt.Errorf("record policy evaluation: %w", err)
	}
	return result, nil
}

func canonicalDocumentMatches(raw, digest []byte, domain canonicaljson.Domain) bool {
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil || len(digest) != sha256.Size {
		return false
	}
	if domain == canonicaljson.DomainIntent {
		if !contractsv1.VerifyIntentDigest(document) {
			return false
		}
		expected, err := contractsv1.IntentDigest(document)
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(expected), []byte("sha256:"+hex.EncodeToString(digest))) == 1
	}
	return canonicaljson.Verify(domain, document, "sha256:"+hex.EncodeToString(digest))
}

func normalizedTarget(intentID string, document map[string]any) string {
	parameters, _ := document["parameters"].(map[string]any)
	if candidate, ok := parameters["target"].(string); ok {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && len(candidate) <= 256 {
			for _, r := range candidate {
				if unicode.IsControl(r) {
					return intentID
				}
			}
			return candidate
		}
	}
	return intentID
}

func documentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func documentInt(document map[string]any, key string) int {
	value, _ := document[key].(float64)
	return int(value)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func nullableID(value string) any {
	if value == "" {
		return nil
	}
	return value
}
