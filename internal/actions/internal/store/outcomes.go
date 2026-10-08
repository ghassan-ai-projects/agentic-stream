package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const insertOutcomeSQL = `
		INSERT INTO outcomes (
			outcome_id, command_id, ordinal, status, provider_result_json,
			observed_effect_json, reconciliation_status, outcome_sha256, traceparent, tracestate, occurred_at
		) VALUES (?, ?, (SELECT COALESCE(MAX(ordinal), 0) + 1 FROM outcomes WHERE command_id = ?), ?, ?, ?, ?, ?, ?, ?, ?)`

const closeDispatchCommandSQL = `
		UPDATE commands SET status = ?, updated_at = ? WHERE command_id = ? AND status = 'dispatching'`

const closeDispatchOutboxSQL = `
		UPDATE outbox SET status = ?, lease_owner = NULL, lease_until = NULL,
			last_error_code = ?, delivered_at = CASE WHEN ? = 'delivered' THEN ? ELSE delivered_at END
		WHERE outbox_id = ? AND lease_owner = ?`

const storeVerificationSQL = `
		INSERT INTO verifications (verification_id, intent_id, command_id, outcome_id, status, updated_at)
		SELECT ?, intent_id, command_id, ?, ?, ? FROM commands WHERE command_id = ?
		ON CONFLICT(intent_id) DO UPDATE SET outcome_id = excluded.outcome_id,
			status = excluded.status, updated_at = excluded.updated_at`

// InsertOutcome appends the next ordinal outcome to a command's ledger.
func (tx *Tx) InsertOutcome(ctx context.Context, outcome domain.OutcomeRecord) error {
	providerJSON, err := optionalJSON(outcome.ProviderResult)
	if err != nil {
		return fmt.Errorf("encode provider result: %w", err)
	}
	observedJSON, err := optionalJSON(outcome.ObservedEffect)
	if err != nil {
		return fmt.Errorf("encode observed effect: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, insertOutcomeSQL,
		outcome.ID, outcome.CommandID, outcome.CommandID, outcome.Status, providerJSON, observedJSON,
		outcome.Reconciliation, outcome.SHA, storage.NullIfEmpty(outcome.Trace.Traceparent), storage.NullIfEmpty(outcome.Trace.Tracestate), formatTime(outcome.At)); err != nil {
		return fmt.Errorf("record action outcome: %w", err)
	}
	return nil
}

// CloseDispatch moves the command, outbox and verification rows to the states
// the dispatch result implies.
func (tx *Tx) CloseDispatch(ctx context.Context, closure domain.DispatchClosure) error {
	command, result, at := closure.Leased.Command, closure.Result, formatTime(closure.At)
	if _, err := tx.tx.ExecContext(ctx, closeDispatchCommandSQL, result.CommandStatus, at, command.CommandID); err != nil {
		return fmt.Errorf("record command status: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, closeDispatchOutboxSQL,
		result.OutboxStatus, storage.NullIfEmpty(result.ErrorCode), result.OutboxStatus, at, closure.Leased.OutboxID, closure.Leased.LeaseOwner); err != nil {
		return fmt.Errorf("finish command outbox: %w", err)
	}
	if _, err := tx.tx.ExecContext(ctx, storeVerificationSQL,
		closure.VerificationID, closure.OutcomeID, result.VerificationStatus, at, command.CommandID); err != nil {
		return fmt.Errorf("record command verification: %w", err)
	}
	return nil
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
