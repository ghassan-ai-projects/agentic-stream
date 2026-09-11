package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EvaluateIntent runs the ordered policy gates and atomically creates either
// an approval request or a Command plus outbox row.
func (g *Gateway) EvaluateIntent(ctx context.Context, tx *sql.Tx, intentID string, now time.Time) (Result, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return Result{IntentID: intentID}, err
	}
	row, err := g.loadIntent(ctx, tx, intentID)
	if err != nil {
		return Result{IntentID: intentID}, err
	}
	if err := g.assertPolicyEpoch(ctx, row); err != nil {
		return g.finish(ctx, tx, row, Result{IntentID: row.IntentID, DecisionID: row.DecisionID}, "denied", "epoch_killed", now)
	}
	result := Result{IntentID: row.IntentID, DecisionID: row.DecisionID}
	if row.PolicyStatus != "pending" {
		return g.evaluateExisting(ctx, tx, row, result, now)
	}
	return g.evaluatePending(ctx, tx, row, result, now)
}

func (g *Gateway) assertPolicyEpoch(ctx context.Context, row intentRow) error {
	if g.epochControl == nil || row.PolicyEpoch == "" {
		return nil
	}
	if err := g.epochControl.AssertDecision(ctx, row.PolicyEpoch); err != nil {
		return fmt.Errorf("assert policy epoch: %w", err)
	}
	return nil
}

func (g *Gateway) evaluateExisting(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, error) {
	if row.PolicyStatus == "approval_required" {
		if result, expired, err := g.expireExistingApproval(ctx, tx, row, result, now); expired || err != nil {
			return result, err
		}
	}
	result.Result = row.PolicyStatus
	result.Reason = "already_evaluated"
	if row.PolicyStatus == "approved" {
		if err := tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", row.IntentID).Scan(&result.CommandID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return result, fmt.Errorf("load existing command id: %w", err)
		}
	}
	if row.PolicyStatus == "approval_required" {
		if err := tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&result.ApprovalID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return result, fmt.Errorf("load pending approval id: %w", err)
		}
	}
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func (g *Gateway) expireExistingApproval(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, bool, error) {
	var approvalID, expiry string
	if err := tx.QueryRowContext(ctx, "SELECT approval_id, expires_at FROM approvals WHERE intent_id = ? AND status = 'pending'", row.IntentID).Scan(&approvalID, &expiry); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, false, nil
		}
		return result, false, fmt.Errorf("load pending approval for expiry: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiry)
	if err == nil && expiresAt.After(now) {
		return result, false, nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE approvals SET status = 'expired' WHERE intent_id = ? AND status = 'pending'", row.IntentID); err != nil {
		return result, true, fmt.Errorf("expire approval: %w", err)
	}
	if err := appendApprovalResolved(ctx, tx, row, approvalID, "expired", "approval_expired", now); err != nil {
		return result, true, err
	}
	result, err = g.finish(ctx, tx, row, result, "expired", "approval_expired", now)
	return result, true, err
}

func (g *Gateway) evaluatePending(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, error) {
	if row.ValidationStatus != "accepted" {
		return g.finish(ctx, tx, row, result, "denied", "decision_not_accepted", now)
	}

	decision, reason := decodeDocument(row.DecisionJSON, contractsv1.SchemaDecision)
	if reason != "" {
		return g.finish(ctx, tx, row, result, "denied", reason, now)
	}
	if !matchesDecisionIdentity(row, decision) {
		return g.finish(ctx, tx, row, result, "denied", "identity_mismatch", now)
	}
	if !canonicalDocumentMatches(row.DecisionJSON, row.DecisionSHA, canonicaljson.DomainDecision) {
		return g.finish(ctx, tx, row, result, "denied", "decision_digest_mismatch", now)
	}

	intent, reason := decodeDocument(row.IntentJSON, contractsv1.SchemaIntent)
	if reason != "" {
		return g.finish(ctx, tx, row, result, "denied", reason, now)
	}
	if !canonicalDocumentMatches(row.IntentJSON, row.IntentSHA, canonicaljson.DomainIntent) {
		return g.finish(ctx, tx, row, result, "denied", "intent_digest_mismatch", now)
	}
	if reason, err := g.compensationFailure(ctx, tx, row, intent); err != nil {
		return result, err
	} else if reason != "" {
		return g.finish(ctx, tx, row, result, "denied", reason, now)
	}
	if !matchesIntentIdentity(row, intent) {
		return g.finish(ctx, tx, row, result, "denied", "identity_mismatch", now)
	}
	if !episodeConcluded(row) {
		return g.finish(ctx, tx, row, result, "denied", "episode_not_concluded", now)
	}
	if row.CurrentSituation != row.SituationVersion {
		return g.markStale(ctx, tx, row, result, now)
	}
	if sourceHealthIncomplete(row) {
		return g.finish(ctx, tx, row, result, "denied", "source_health_incomplete", now)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expiresAt.After(now) {
		return g.finish(ctx, tx, row, result, "expired", "intent_expired", now)
	}
	return g.routeIntent(ctx, tx, row, intent, result, expiresAt, now)
}

func decodeDocument(raw []byte, schema contractsv1.SchemaName) (map[string]any, string) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, "schema_invalid"
	}
	if err := contractsv1.Validate(schema, document); err != nil {
		return nil, "schema_invalid"
	}
	return document, ""
}

func matchesDecisionIdentity(row intentRow, document map[string]any) bool {
	return documentString(document, "decision_id") == row.DecisionID &&
		documentString(document, "episode_id") == row.EpisodeID &&
		documentString(document, "situation_id") == row.SituationID &&
		documentInt(document, "situation_version") == row.SituationVersion &&
		row.EpisodeTenant == row.TenantID && row.SituationTenant == row.TenantID &&
		row.DecisionSituation == row.SituationID && row.DecisionVersion == row.SituationVersion &&
		row.EpisodeSituation == row.SituationID && row.EpisodeVersion == row.SituationVersion
}

func matchesIntentIdentity(row intentRow, document map[string]any) bool {
	return documentString(document, "intent_id") == row.IntentID &&
		documentString(document, "decision_id") == row.DecisionID &&
		documentString(document, "tenant_id") == row.TenantID &&
		documentString(document, "situation_id") == row.SituationID &&
		documentInt(document, "situation_version") == row.SituationVersion &&
		documentString(document, "type") == row.IntentType &&
		documentString(document, "risk_class") == row.RiskClass
}

func (g *Gateway) compensationFailure(ctx context.Context, tx *sql.Tx, row intentRow, document map[string]any) (string, error) {
	compensates, ok := document["compensates"].(string)
	if !ok || compensates == "" {
		return "", nil
	}
	var commandTenant string
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id FROM commands WHERE command_id = ?", compensates).Scan(&commandTenant); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "compensation_target_missing", nil
		}
		return "", fmt.Errorf("load compensation target: %w", err)
	}
	if commandTenant != row.TenantID {
		return "compensation_tenant_mismatch", nil
	}
	return "", nil
}

func episodeConcluded(row intentRow) bool {
	return row.EpisodeLifecycle == "concluded" || row.EpisodeLifecycle == "closed"
}

func sourceHealthIncomplete(row intentRow) bool {
	consequential := row.RiskClass == "R2" || row.RiskClass == "R3" || row.RiskClass == "R4"
	return consequential && (row.CurrentCompleteness == "provisional" || row.CurrentCompleteness == "uncertain")
}

func (g *Gateway) markStale(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, error) {
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'stale', updated_at = ? WHERE intent_id = ?", formatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("mark stale intent: %w", err)
	}
	return g.audit(ctx, tx, row, result, "stale", "situation_version_stale", now)
}

func (g *Gateway) routeIntent(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	// The risk-class policy document is authoritative and enforced first: R3/R4
	// are unconditionally denied regardless of a catalog requires_approval flag.
	// requires_approval is an override only for the auto-approvable R0/R1 tier,
	// and it consults an already-approved approval before creating a new request
	// so a resolved approval terminates in dispatch instead of spawning another
	// approval on the next EvaluateIntent (the approve -> re-pending loop).
	switch row.RiskClass {
	case "R0", "R1":
		if row.RequiresApproval != 0 {
			return g.approveOrRequireApproval(ctx, tx, row, intent, result, expiresAt, now)
		}
		return g.approveAutomatic(ctx, tx, row, intent, result, now)
	case "R2":
		return g.routeConsequentialIntent(ctx, tx, row, intent, result, expiresAt, now)
	case "R3", "R4":
		return g.finish(ctx, tx, row, result, "denied", "risk_policy_denied", now)
	default:
		return g.finish(ctx, tx, row, result, "denied", "unknown_risk_class", now)
	}
}

func (g *Gateway) routeConsequentialIntent(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	if g.calibration != nil && row.SituationType != "" && row.ExecutorVersion != "" {
		if err := g.calibration.AssertCalibration(ctx, tx, storage.CalibrationArtifact{
			Domain: row.SituationType, ModelRevision: row.ExecutorVersion,
		}); err == nil {
			return g.approveAutomatic(ctx, tx, row, intent, result.WithReason("calibrated_automation"), now)
		}
	}
	return g.approveOrRequireApproval(ctx, tx, row, intent, result, expiresAt, now)
}

// approveOrRequireApproval dispatches when a human has already approved this
// intent, otherwise it opens a fresh approval request. Consulting the existing
// approval is what breaks the approve -> re-pending -> new-approval loop: once
// ResolveApproval marks the approval 'approved' and re-runs EvaluateIntent,
// this path finds that row and terminates in dispatch.
func (g *Gateway) approveOrRequireApproval(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, expiresAt, now time.Time) (Result, error) {
	var approvedApproval string
	err := tx.QueryRowContext(ctx, "SELECT approval_id FROM approvals WHERE intent_id = ? AND status = 'approved' ORDER BY decided_at DESC LIMIT 1", row.IntentID).Scan(&approvedApproval)
	switch {
	case err == nil:
		result.ApprovalID = approvedApproval
		result.Reason = "approved_by_human"
		return g.approveAutomatic(ctx, tx, row, intent, result, now)
	case errors.Is(err, sql.ErrNoRows):
		return g.requireApproval(ctx, tx, row, intent, result, expiresAt, now)
	default:
		return result, fmt.Errorf("load approved approval: %w", err)
	}
}
