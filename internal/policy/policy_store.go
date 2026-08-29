package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

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
	var traceparent, tracestate sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT i.intent_id, i.decision_id, i.tenant_id, i.situation_id,
		       i.situation_version, i.intent_type, i.risk_class, i.intent_json,
		       i.intent_sha256, i.expires_at, i.policy_status, i.rate_limit_per_hour, i.requires_approval,
		       d.validation_status, d.raw_json, d.decision_sha256,
		       d.situation_id, d.situation_version, d.traceparent, d.tracestate,
		       e.episode_id, e.tenant_id, e.situation_id, e.situation_version,
		       e.lifecycle_status, e.executor_version, e.policy_epoch, s.tenant_id, s.current_version, s.situation_type,
		       COALESCE((SELECT sv.completeness FROM situation_versions sv WHERE sv.situation_id = s.situation_id AND sv.version = s.current_version), '')
		FROM intents i
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN episodes e ON e.episode_id = d.episode_id
		JOIN situations s ON s.situation_id = i.situation_id
		WHERE i.intent_id = ?`, intentID,
	).Scan(
		&row.IntentID, &row.DecisionID, &row.TenantID, &row.SituationID,
		&row.SituationVersion, &row.IntentType, &row.RiskClass, &row.IntentJSON,
		&row.IntentSHA, &row.ExpiresAt, &row.PolicyStatus, &row.RateLimitPerHour, &row.RequiresApproval,
		&row.ValidationStatus, &row.DecisionJSON, &row.DecisionSHA,
		&row.DecisionSituation, &row.DecisionVersion, &traceparent, &tracestate,
		&row.EpisodeID, &row.EpisodeTenant, &row.EpisodeSituation, &row.EpisodeVersion,
		&row.EpisodeLifecycle, &row.ExecutorVersion, &row.PolicyEpoch, &row.SituationTenant, &row.CurrentSituation, &row.SituationType, &row.CurrentCompleteness,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return row, fmt.Errorf("intent %s not found", intentID)
	}
	if err != nil {
		return row, fmt.Errorf("load intent %s: %w", intentID, err)
	}
	row.Traceparent = traceparent.String
	row.Tracestate = tracestate.String
	return row, nil
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

func appendApprovalWithdrawn(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, reason string, now time.Time) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.withdrawn:"+approvalID, row.TenantID, notify.TypeApprovalWithdrawn, "approval/"+approvalID, row.SituationID, map[string]any{
		"tenant_id": row.TenantID, "approval_id": approvalID, "intent_id": row.IntentID, "situation_id": row.SituationID,
		"situation_version": row.SituationVersion, "reason": reason, "source_authority": notify.SourceForTenant(row.TenantID),
	}, now, traceContext(row)); err != nil {
		return fmt.Errorf("append approval withdrawn notification: %w", err)
	}
	return nil
}

func appendApprovalResolved(ctx context.Context, tx *sql.Tx, row intentRow, approvalID, status, reason string, now time.Time) error {
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "approval.resolved:"+approvalID+":"+status, row.TenantID, notify.TypeApprovalResolved, "approval/"+approvalID, row.SituationID, map[string]any{
		"tenant_id": row.TenantID, "approval_id": approvalID, "intent_id": row.IntentID, "decision_id": row.DecisionID,
		"situation_id": row.SituationID, "situation_version": row.SituationVersion,
		"status": status, "reason": reason, "source_authority": notify.SourceForTenant(row.TenantID),
	}, now, traceContext(row)); err != nil {
		return fmt.Errorf("append approval resolved notification: %w", err)
	}
	return nil
}

func traceContext(row intentRow) contractsv1.TraceContext {
	return contractsv1.TraceContext{Traceparent: row.Traceparent, Tracestate: row.Tracestate}
}

func (g *Gateway) dispatchWithinLimit(ctx context.Context, tx *sql.Tx, row intentRow, now time.Time) (bool, error) {
	bucket := now.UTC().Format("2006-01-02T15:00")
	var count int
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO intent_dispatch_counts (tenant_id, intent_type, bucket, count)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(tenant_id, intent_type, bucket) DO UPDATE SET count = count + 1
		RETURNING count`,
		row.TenantID, row.IntentType, bucket,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("increment intent dispatch counter: %w", err)
	}
	return count > row.RateLimitPerHour, nil
}

func nullableID(value string) any {
	if value == "" {
		return nil
	}
	return value
}
