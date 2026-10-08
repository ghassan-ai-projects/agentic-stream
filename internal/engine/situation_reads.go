package engine

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SituationSummary is a Situation's current row as an operator sees it.
type SituationSummary = domain.SituationSummary

// SituationVersionRecord is one published Situation version with the
// evidence set (lineage) it was derived from.
type SituationVersionRecord = domain.SituationVersionRecord

// ListSituations reads the tenant's Situations, newest evidence first,
// optionally for one entity. It only reads.
func ListSituations(ctx context.Context, db *storage.DB, tenantID, entityID string) ([]SituationSummary, error) {
	return app.ListSituations(ctx, store.NewReader(db), tenantID, entityID)
}

// SituationVersion reads one version of a tenant's Situation with its
// evidence set; version 0 means the current version. It only reads.
func SituationVersion(ctx context.Context, db *storage.DB, tenantID, situationID string, version int) (SituationVersionRecord, error) {
	return app.SituationVersion(ctx, store.NewReader(db), tenantID, situationID, version)
}
