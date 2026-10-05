package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// RowQueryer is the one-row read surface both an open transaction and the
// database satisfy.
type RowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// EpisodeLifecycle reads an episode's durable lifecycle status.
func EpisodeLifecycle(ctx context.Context, q RowQueryer, episodeID string) (string, error) {
	var lifecycle string
	if err := q.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err != nil {
		return "", fmt.Errorf("read episode lifecycle: %w", err)
	}
	return lifecycle, nil
}

// EpisodeSupersededNow reports whether the episode is already superseded,
// polled outside a transaction by the supersession watch. Read errors are the
// caller's to ignore; the poll retries.
func EpisodeSupersededNow(ctx context.Context, db *storage.DB, episodeID string) bool {
	var lifecycle string
	return db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle) == nil &&
		lifecycle == string(episodeledger.LifecycleSuperseded)
}

// AttemptStatus reads an attempt's current lifecycle status.
func AttemptStatus(ctx context.Context, tx *sql.Tx, identity episodeledger.Identity) (episodeledger.AttemptStatus, error) {
	var current episodeledger.AttemptStatus
	if err := tx.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&current); err != nil {
		return "", fmt.Errorf("read episode attempt status: %w", err)
	}
	return current, nil
}

// AnnotateRejectedDecision records why a decision was rejected.
func AnnotateRejectedDecision(ctx context.Context, tx *sql.Tx, decisionID, reason string) error {
	if _, err := tx.ExecContext(ctx, "UPDATE decisions SET rejection_reason = ? WHERE decision_id = ?", reason, decisionID); err != nil {
		return fmt.Errorf("annotate rejected decision: %w", err)
	}
	return nil
}

// AcceptDecision marks a governed decision accepted.
func AcceptDecision(ctx context.Context, tx *sql.Tx, decisionID string) error {
	if _, err := tx.ExecContext(ctx, "UPDATE decisions SET validation_status = 'accepted' WHERE decision_id = ?", decisionID); err != nil {
		return fmt.Errorf("accept decision: %w", err)
	}
	return nil
}

// RetainForRetry keeps a failed episode running for another attempt.
func RetainForRetry(ctx context.Context, tx *sql.Tx, episodeID string) error {
	if err := episodeledger.RetainForRetry(ctx, tx, episodeID); err != nil {
		return fmt.Errorf("update episode failed: %w", err)
	}
	return nil
}

// ConcludeEpisode concludes an episode with its terminal document.
func ConcludeEpisode(ctx context.Context, tx *sql.Tx, episodeID, now string, terminalJSON []byte) error {
	if err := episodeledger.Conclude(ctx, tx, episodeID, now, terminalJSON); err != nil {
		return fmt.Errorf("update episode terminal: %w", err)
	}
	return nil
}
