package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

// RecordShadowComparison persists one sealed shadow trial in its own
// transaction. The unique comparison key prevents a replay retry from
// overwriting an earlier artifact; the row never enters intents, commands or
// the outbox.
func (s Store) RecordShadowComparison(ctx context.Context, comparison domain.Comparison) error {
	return s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, insertShadowComparisonSQL, columnValues(comparison)...); err != nil {
			return fmt.Errorf("record shadow comparison: %w", err)
		}
		return nil
	})
}

const insertShadowComparisonSQL = `
		INSERT INTO shadow_comparisons (
			comparison_id, comparison_key, tenant_id, episode_id, situation_id,
			situation_version, trigger_id, snapshot_sha256, spec_sha256, policy_sha256,
			baseline_executor_version, tamoz_executor_version,
			baseline_manifest_sha256, tamoz_manifest_sha256,
			baseline_decision_json, baseline_decision_sha256,
			tamoz_decision_json, tamoz_decision_sha256,
			comparison_json, comparison_sha256, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// columnValues lists the comparison in insertShadowComparisonSQL column order.
func columnValues(c domain.Comparison) []any {
	return []any{
		c.ComparisonID, c.ComparisonKey, c.TenantID, c.EpisodeID, c.SituationID,
		c.SituationVersion, c.TriggerID, c.SnapshotSHA256, c.SpecSHA256, c.PolicySHA256,
		c.BaselineExecutorVersion, c.TamozExecutorVersion,
		c.BaselineManifestSHA256, c.TamozManifestSHA256,
		c.BaselineDecisionJSON, c.BaselineDecisionSHA256,
		c.TamozDecisionJSON, c.TamozDecisionSHA256,
		c.ComparisonJSON, c.ComparisonSHA256, kernel.FormatTime(c.CreatedAt),
	}
}
