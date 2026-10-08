package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
)

// ListSituations reads the tenant's Situations, newest evidence first.
func ListSituations(ctx context.Context, reader store.Reader, tenantID, entityID string) ([]domain.SituationSummary, error) {
	return reader.ListSituations(ctx, tenantID, entityID) //nolint:wrapcheck // The store names the failed read.
}

// SituationVersion reads one version of a tenant's Situation with its
// evidence set; version 0 means the current version.
func SituationVersion(ctx context.Context, reader store.Reader, tenantID, situationID string, version int) (domain.SituationVersionRecord, error) {
	return reader.SituationVersion(ctx, tenantID, situationID, version) //nolint:wrapcheck // The store names the failed read.
}
