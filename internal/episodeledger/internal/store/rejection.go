package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
)

// EpisodeExists reports whether the ledger knows the episode.
func (t *Tx) EpisodeExists(ctx context.Context, episodeID string) (bool, error) {
	var exists int
	err := t.q.QueryRowContext(ctx, "SELECT 1 FROM episodes WHERE episode_id = ?", episodeID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check rejected episode: %w", err)
	}
	return true, nil
}

// InsertRejection records a rejection once; a repeat of the same id is ignored.
// The episode is stored only when known, so an unknown episode keeps a null
// foreign key; the attempt is stored only when named.
func (t *Tx) InsertRejection(ctx context.Context, rejection domain.Rejection, episodeKnown bool) error {
	var episode, attempt any
	if episodeKnown {
		episode = rejection.EpisodeID
	}
	if rejection.AttemptID != "" {
		attempt = rejection.AttemptID
	}
	_, err := t.q.ExecContext(ctx, insertWorkerRejectionSQL, rejection.ID, episode, attempt, rejection.Fence, rejection.Reason, rejection.Details, rejection.At)
	if err != nil {
		return fmt.Errorf("record rejection: %w", err)
	}
	return nil
}

const insertWorkerRejectionSQL = `
 INSERT INTO episode_rejections (
 rejection_id, episode_id, attempt_id, fence, reason, details_json, created_at
 ) VALUES (?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT (rejection_id) DO NOTHING`
