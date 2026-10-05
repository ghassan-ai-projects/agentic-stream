package episodes

import (
	"context"
	"database/sql"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// loadReconsideration loads the reconsideration evidence and assembles the
// reconsideration document for the request.
func loadReconsideration(ctx context.Context, tx *sql.Tx, item schedulerItem, ev evaluation, delta, snapshot map[string]any) (map[string]any, error) {
	supersededVersion := domain.SnapshotInt(delta, "superseded_version")
	invalidatedCommandID := domain.SnapshotString(delta, "invalidated_command_id")
	scanned, err := store.LoadReconsideration(ctx, tx, item, supersededVersion, invalidatedCommandID)
	if err != nil {
		return nil, err
	}
	return scanned.ReconsiderationDocument(store.Evaluation(ev), delta, snapshot)
}
