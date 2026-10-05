package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

// ReadReservation loads a call without interpreting its authorization or result.
func (tx *Tx) ReadReservation(ctx context.Context, key domain.ReservationKey) (*domain.ReservationRow, error) {
	var row domain.ReservationRow
	var count, size sql.NullInt64
	err := tx.tx.QueryRowContext(ctx, `SELECT request_sha256, token_id, runtime_epoch, status, result_json, result_sha256, row_count, result_bytes FROM evidence_call_ledger WHERE tenant_id = ? AND episode_id = ? AND attempt_id = ? AND fence = ? AND call_id = ?`, key.TenantID, key.EpisodeID, key.AttemptID, key.Fence, key.CallID).Scan(&row.RequestHash, &row.TokenID, &row.Epoch, &row.Status, &row.ResultJSON, &row.ResultHash, &count, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load evidence call: %w", err)
	}
	row.RowCount = count.Int64
	row.ResultBytes = size.Int64
	return &row, nil
}

// LiveEpisode loads the tenant-bound episode at reservation time.
func (tx *Tx) LiveEpisode(ctx context.Context, call domain.Call) (domain.EpisodeState, error) {
	var state domain.EpisodeState
	err := tx.tx.QueryRowContext(ctx, `SELECT lifecycle_status, COALESCE(current_attempt_id, ''), current_fence FROM episodes WHERE episode_id = ? AND tenant_id = ?`, call.EpisodeID, call.TenantID).Scan(&state.Lifecycle, &state.AttemptID, &state.Fence)
	if errors.Is(err, sql.ErrNoRows) {
		return state, fmt.Errorf("evidence episode is unknown")
	}
	if err != nil {
		return state, fmt.Errorf("load evidence episode: %w", err)
	}
	return state, nil
}

// LiveAttempt loads the exact fenced attempt status.
func (tx *Tx) LiveAttempt(ctx context.Context, call domain.Call) (string, error) {
	var status string
	if err := tx.tx.QueryRowContext(ctx, `SELECT status FROM episode_attempts WHERE episode_id = ? AND attempt_id = ? AND fence = ?`, call.EpisodeID, call.AttemptID, call.Fence).Scan(&status); err != nil {
		return "", fmt.Errorf("load evidence attempt: %w", err)
	}
	return status, nil
}

// CompletionEpisode loads the current binding without changing the original query.
func (tx *Tx) CompletionEpisode(ctx context.Context, key domain.ReservationKey) (domain.EpisodeState, error) {
	var state domain.EpisodeState
	if err := tx.tx.QueryRowContext(ctx, `SELECT lifecycle_status, COALESCE(current_attempt_id, ''), current_fence FROM episodes WHERE episode_id = ?`, key.EpisodeID).Scan(&state.Lifecycle, &state.AttemptID, &state.Fence); err != nil {
		return state, fmt.Errorf("load completion episode: %w", err)
	}
	return state, nil
}

// CompletionAttempt loads the reserved fenced attempt at conclusion.
func (tx *Tx) CompletionAttempt(ctx context.Context, key domain.ReservationKey) (string, error) {
	var status string
	if err := tx.tx.QueryRowContext(ctx, `SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?`, key.AttemptID, key.EpisodeID, key.Fence).Scan(&status); err != nil {
		return "", fmt.Errorf("load completion attempt: %w", err)
	}
	return status, nil
}
