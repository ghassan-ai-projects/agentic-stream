package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// EpisodeExists reports whether the ledger knows the episode.
func (t *Tx) EpisodeExists(ctx context.Context, episodeID string) (bool, error) {
	_, exists, err := storage.QueryOptional[int](ctx, t.q, "SELECT 1 FROM episodes WHERE episode_id = ?", episodeID)
	if err != nil {
		return false, fmt.Errorf("check rejected episode: %w", err)
	}
	return exists, nil
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
