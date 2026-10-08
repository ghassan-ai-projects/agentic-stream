package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
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
	state, err := tx.episodeState(ctx, `SELECT lifecycle_status, COALESCE(current_attempt_id, ''), current_fence FROM episodes WHERE episode_id = ? AND tenant_id = ?`, call.EpisodeID, call.TenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return state, fmt.Errorf("evidence episode is unknown")
	}
	if err != nil {
		return state, fmt.Errorf("load evidence episode: %w", err)
	}
	return state, nil
}

// LiveAttempt loads the exact fenced attempt status.
func (tx *Tx) LiveAttempt(ctx context.Context, call domain.Call) (bool, error) {
	return tx.attemptInFlight(ctx, "load evidence attempt", call.EpisodeID, call.AttemptID, call.Fence)
}

// CompletionEpisode loads the current binding without changing the original query.
func (tx *Tx) CompletionEpisode(ctx context.Context, key domain.ReservationKey) (domain.EpisodeState, error) {
	state, err := tx.episodeState(ctx, `SELECT lifecycle_status, COALESCE(current_attempt_id, ''), current_fence FROM episodes WHERE episode_id = ?`, key.EpisodeID)
	if err != nil {
		return state, fmt.Errorf("load completion episode: %w", err)
	}
	return state, nil
}

// CompletionAttempt loads the reserved fenced attempt at conclusion.
func (tx *Tx) CompletionAttempt(ctx context.Context, key domain.ReservationKey) (bool, error) {
	return tx.attemptInFlight(ctx, "load completion attempt", key.EpisodeID, key.AttemptID, key.Fence)
}

func (tx *Tx) episodeState(ctx context.Context, query string, args ...any) (domain.EpisodeState, error) {
	var state domain.EpisodeState
	var lifecycle episodeledger.LifecycleStatus
	if err := tx.tx.QueryRowContext(ctx, query, args...).Scan(&lifecycle, &state.AttemptID, &state.Fence); err != nil {
		return state, err //nolint:wrapcheck // Each caller wraps with its operation and distinguishes no rows.
	}
	state.Closed = lifecycle.Closed()
	state.Running = lifecycle == episodeledger.LifecycleRunning
	return state, nil
}

func (tx *Tx) attemptInFlight(ctx context.Context, failure, episodeID, attemptID string, fence int64) (bool, error) {
	var status episodeledger.AttemptStatus
	if err := tx.tx.QueryRowContext(ctx, `SELECT status FROM episode_attempts WHERE episode_id = ? AND attempt_id = ? AND fence = ?`, episodeID, attemptID, fence).Scan(&status); err != nil {
		return false, fmt.Errorf("%s: %w", failure, err)
	}
	return status.InFlight(), nil
}
