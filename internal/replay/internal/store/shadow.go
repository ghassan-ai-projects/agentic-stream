package store

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// RecordShadowComparison persists one sealed shadow trial through the
// qualification-owned comparison store in its own transaction.
func (s Store) RecordShadowComparison(ctx context.Context, comparison domain.Comparison) error {
	comparisons := qualification.ShadowComparisonStore{}
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return comparisons.Record(ctx, tx, qualificationRecord(comparison))
	})
}

func qualificationRecord(comparison domain.Comparison) qualification.ShadowComparison {
	return qualification.ShadowComparison{
		ComparisonID: comparison.ComparisonID, ComparisonKey: comparison.ComparisonKey, TenantID: comparison.TenantID,
		EpisodeID: comparison.EpisodeID, SituationID: comparison.SituationID, SituationVersion: comparison.SituationVersion,
		TriggerID: comparison.TriggerID, SnapshotSHA256: comparison.SnapshotSHA256, SpecSHA256: comparison.SpecSHA256, PolicySHA256: comparison.PolicySHA256,
		BaselineExecutorVersion: comparison.BaselineExecutorVersion, TamozExecutorVersion: comparison.TamozExecutorVersion,
		BaselineManifestSHA256: comparison.BaselineManifestSHA256, TamozManifestSHA256: comparison.TamozManifestSHA256,
		BaselineDecisionJSON: comparison.BaselineDecisionJSON, BaselineDecisionSHA256: comparison.BaselineDecisionSHA256,
		TamozDecisionJSON: comparison.TamozDecisionJSON, TamozDecisionSHA256: comparison.TamozDecisionSHA256,
		ComparisonJSON: comparison.ComparisonJSON, ComparisonSHA256: comparison.ComparisonSHA256, CreatedAt: comparison.CreatedAt,
	}
}
