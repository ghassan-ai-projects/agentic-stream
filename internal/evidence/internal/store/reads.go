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
	fence, found, err := episodeledger.ReadEpisodeFence(ctx, tx.tx, call.EpisodeID)
	if err != nil {
		return domain.EpisodeState{}, fmt.Errorf("load evidence episode: %w", err)
	}
	if !found || fence.TenantID != call.TenantID {
		return domain.EpisodeState{}, fmt.Errorf("evidence episode is unknown")
	}
	return episodeState(fence, call.EpisodeID, call.AttemptID, call.Fence), nil
}

// LiveAttempt loads the exact fenced attempt status.
func (tx *Tx) LiveAttempt(ctx context.Context, call domain.Call) (bool, error) {
	return tx.attemptInFlight(ctx, "load evidence attempt", call.EpisodeID, call.AttemptID, call.Fence)
}

// CompletionEpisode loads the current binding without changing the original query.
func (tx *Tx) CompletionEpisode(ctx context.Context, key domain.ReservationKey) (domain.EpisodeState, error) {
	fence, _, err := episodeledger.ReadEpisodeFence(ctx, tx.tx, key.EpisodeID)
	if err != nil {
		return domain.EpisodeState{}, fmt.Errorf("load completion episode: %w", err)
	}
	return episodeState(fence, key.EpisodeID, key.AttemptID, key.Fence), nil
}

// CompletionAttempt loads the reserved fenced attempt at conclusion.
func (tx *Tx) CompletionAttempt(ctx context.Context, key domain.ReservationKey) (bool, error) {
	return tx.attemptInFlight(ctx, "load completion attempt", key.EpisodeID, key.AttemptID, key.Fence)
}

func episodeState(fence episodeledger.EpisodeFence, episodeID, attemptID string, attemptFence int64) domain.EpisodeState {
	identity := episodeledger.Identity{EpisodeID: episodeID, AttemptID: attemptID, Fence: attemptFence}
	return domain.EpisodeState{
		Current: fence.CheckIdentity(identity) == nil,
		Closed:  fence.Lifecycle.Closed(),
		Running: fence.Lifecycle == episodeledger.LifecycleRunning,
	}
}

func (tx *Tx) attemptInFlight(ctx context.Context, failure, episodeID, attemptID string, fence int64) (bool, error) {
	status, err := episodeledger.ReadAttemptStatus(ctx, tx.tx, episodeledger.Identity{EpisodeID: episodeID, AttemptID: attemptID, Fence: fence})
	if err != nil {
		return false, fmt.Errorf("%s: %w", failure, err)
	}
	return status.InFlight(), nil
}
