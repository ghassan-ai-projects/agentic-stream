package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const insertDecisionSQL = `
		INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
			situation_version, raw_json, decision_sha256, validation_status,
			validation_json, traceparent, tracestate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const insertValidatedIntentSQL = `
		INSERT INTO intents (
			intent_id, decision_id, tenant_id, situation_id, situation_version,
			intent_type, risk_class, intent_json, intent_sha256, expires_at,
			rate_limit_per_hour, requires_approval, policy_status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`

// DecisionInsert is one decision record bound to its episode attempt.
type DecisionInsert struct {
	DecisionID       string
	EpisodeID        string
	AttemptID        string
	Fence            int64
	SituationID      string
	SituationVersion int
	RawJSON          []byte
	Digest           []byte
	ValidationStatus string
	ValidationJSON   []byte
	Traceparent      string
	Tracestate       string
	Now              string
}

// InsertDecision allocates the episode's next decision ordinal and inserts
// the decision record inside the caller's transaction.
func InsertDecision(ctx context.Context, tx *Tx, row DecisionInsert) error {
	var ordinal int
	if err := tx.tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(ordinal), 0) + 1 FROM decisions WHERE episode_id = ?", row.EpisodeID).Scan(&ordinal); err != nil {
		return fmt.Errorf("allocate decision ordinal: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, insertDecisionSQL,
		row.DecisionID, row.EpisodeID, row.AttemptID, row.Fence, ordinal,
		row.SituationID, row.SituationVersion,
		row.RawJSON, row.Digest, row.ValidationStatus, row.ValidationJSON,
		storage.NullIfEmpty(row.Traceparent), storage.NullIfEmpty(row.Tracestate), row.Now,
	); err != nil {
		return fmt.Errorf("insert decision: %w", err)
	}
	return nil
}

// ValidatedIntentInsert is one active-mode validated intent.
type ValidatedIntentInsert struct {
	Intent           decisions.Intent
	DecisionID       string
	TenantID         string
	SituationID      string
	SituationVersion int
	Now              string
}

// InsertValidatedIntent inserts one validated intent (policy_status pending,
// never governed) inside the caller's transaction.
func InsertValidatedIntent(ctx context.Context, tx *Tx, row ValidatedIntentInsert) error {
	digest, err := canonicaljson.DecodeDigest(row.Intent.Digest)
	if err != nil {
		return fmt.Errorf("decode intent digest: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, insertValidatedIntentSQL,
		row.Intent.ID, row.DecisionID, row.TenantID, row.SituationID, row.SituationVersion,
		row.Intent.Type, row.Intent.RiskClass, row.Intent.CanonicalJSON, digest,
		sources.FormatTime(row.Intent.ExpiresAt), row.Intent.RateLimitPerHour,
		storage.BoolInt(row.Intent.RequiresApproval), row.Now, row.Now,
	); err != nil {
		return fmt.Errorf("insert intent %s: %w", row.Intent.ID, err)
	}
	return nil
}
