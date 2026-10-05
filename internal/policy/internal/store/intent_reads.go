package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// LoadIntent reads the accepted intent and its current governance context.
func (tx *Tx) LoadIntent(ctx context.Context, intentID string) (domain.IntentRecord, error) {
	row, err := scanPolicyIntent(tx.tx.QueryRowContext(ctx, loadPolicyIntentSQL, intentID))
	if errors.Is(err, sql.ErrNoRows) {
		return row, fmt.Errorf("intent %s not found", intentID)
	}
	if err != nil {
		return row, fmt.Errorf("load intent %s: %w", intentID, err)
	}
	return row, nil
}

// DispatchWithinLimit increments the hourly dispatch counter or reports a full bucket.
func (tx *Tx) DispatchWithinLimit(ctx context.Context, row domain.IntentRecord, now time.Time) (bool, error) {
	bucket := now.UTC().Format("2006-01-02T15:00")
	var count int
	if err := tx.tx.QueryRowContext(ctx, incrementIntentDispatchSQL,
		row.TenantID, row.IntentType, bucket, row.RateLimitPerHour,
	).Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("increment intent dispatch counter: %w", err)
	}
	return false, nil
}

func nullableID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func scanPolicyIntent(query *sql.Row) (domain.IntentRecord, error) {
	var row domain.IntentRecord
	var traceparent, tracestate sql.NullString
	err := query.Scan(
		&row.IntentID, &row.DecisionID, &row.TenantID, &row.SituationID,
		&row.SituationVersion, &row.IntentType, &row.RiskClass, &row.IntentJSON,
		&row.IntentSHA, &row.ExpiresAt, &row.PolicyStatus, &row.RateLimitPerHour, &row.RequiresApproval,
		&row.ValidationStatus, &row.DecisionJSON, &row.DecisionSHA,
		&row.DecisionSituation, &row.DecisionVersion, &traceparent, &tracestate,
		&row.EpisodeID, &row.EpisodeTenant, &row.EpisodeSituation, &row.EpisodeVersion,
		&row.EpisodeLifecycle, &row.ExecutorVersion, &row.PolicyEpoch, &row.SituationTenant, &row.CurrentSituation, &row.SituationType, &row.CurrentCompleteness,
	)
	if err == nil {
		row.Traceparent = traceparent.String
		row.Tracestate = tracestate.String
	}
	return row, err //nolint:wrapcheck // loadIntent preserves the database error text.
}

const loadPolicyIntentSQL = `
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
		WHERE i.intent_id = ?`

const incrementIntentDispatchSQL = `
		INSERT INTO intent_dispatch_counts (tenant_id, intent_type, bucket, count)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(tenant_id, intent_type, bucket) DO UPDATE SET count = intent_dispatch_counts.count + 1
		WHERE intent_dispatch_counts.count < ?
		RETURNING count`
