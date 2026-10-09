package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// EpisodeLifecycle reads an episode's durable lifecycle status.
func EpisodeLifecycle(ctx context.Context, tx *Tx, episodeID string) (episodeledger.LifecycleStatus, error) {
	fence, found, err := episodeledger.ReadEpisodeFence(ctx, tx.tx, episodeID)
	if err != nil {
		return "", fmt.Errorf("read episode lifecycle: %w", err)
	}
	if !found {
		return "", fmt.Errorf("read episode lifecycle: episode %s: %w", episodeID, sql.ErrNoRows)
	}
	return fence.Lifecycle, nil
}

// EpisodeSupersededNow reports whether the episode is already superseded,
// polled outside a transaction by the supersession watch. Read errors are the
// caller's to ignore; the poll retries.
func (s Store) EpisodeSupersededNow(ctx context.Context, episodeID string) bool {
	lifecycle, err := episodeledger.ReadEpisodeLifecycle(ctx, s.db.DB, episodeID)
	return err == nil && lifecycle == episodeledger.LifecycleSuperseded
}

// AttemptStatus reads an attempt's current lifecycle status.
func AttemptStatus(ctx context.Context, tx *Tx, identity episodeledger.Identity) (episodeledger.AttemptStatus, error) {
	status, err := episodeledger.ReadAttemptStatus(ctx, tx.tx, identity)
	if err != nil {
		return "", fmt.Errorf("read episode attempt status: %w", err)
	}
	return status, nil
}

// AnnotateRejectedDecision records why a decision was rejected.
func AnnotateRejectedDecision(ctx context.Context, tx *Tx, decisionID, reason string) error {
	if _, err := tx.tx.ExecContext(ctx, "UPDATE decisions SET rejection_reason = ? WHERE decision_id = ?", reason, decisionID); err != nil {
		return fmt.Errorf("annotate rejected decision: %w", err)
	}
	return nil
}

// AcceptDecision marks a governed decision accepted.
func AcceptDecision(ctx context.Context, tx *Tx, decisionID string) error {
	if _, err := tx.tx.ExecContext(ctx, "UPDATE decisions SET validation_status = 'accepted' WHERE decision_id = ?", decisionID); err != nil {
		return fmt.Errorf("accept decision: %w", err)
	}
	return nil
}

// RetainForRetry keeps a failed episode running for another attempt.
func RetainForRetry(ctx context.Context, tx *Tx, episodeID string) error {
	if err := episodeledger.RetainForRetry(ctx, tx.tx, episodeID); err != nil {
		return fmt.Errorf("update episode failed: %w", err)
	}
	return nil
}

// ConcludeEpisode concludes an episode with its terminal document.
func ConcludeEpisode(ctx context.Context, tx *Tx, episodeID string, now time.Time, terminalJSON []byte) error {
	if err := episodeledger.Conclude(ctx, tx.tx, episodeID, now, terminalJSON); err != nil {
		return fmt.Errorf("update episode terminal: %w", err)
	}
	return nil
}
