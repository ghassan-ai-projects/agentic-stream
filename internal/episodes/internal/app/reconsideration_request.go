package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// loadReconsideration loads the reconsideration evidence and assembles the
// reconsideration document for the request.
func loadReconsideration(ctx context.Context, tx *store.Tx, item schedulerItem, ev evaluation, delta, snapshot map[string]any) (map[string]any, error) {
	supersededVersion := contractsv1.DocumentInt(delta, "superseded_version")
	invalidatedCommandID := contractsv1.DocumentString(delta, "invalidated_command_id")
	scanned, err := store.LoadReconsideration(ctx, tx, item, supersededVersion, invalidatedCommandID)
	if err != nil {
		return nil, err
	}
	return scanned.ReconsiderationDocument(store.Evaluation(ev), delta, snapshot)
}
