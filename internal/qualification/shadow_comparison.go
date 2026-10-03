package qualification

import (
	"context"
	"database/sql"
	"fmt"
)

// ShadowComparison is an append-only, report-only comparison of two
// Decisions over one immutable Situation snapshot. It is never an approval or
// promotion record and has no action-plane capability.
type ShadowComparison struct {
	ComparisonID            string
	ComparisonKey           string
	TenantID                string
	EpisodeID               string
	SituationID             string
	SituationVersion        int
	TriggerID               string
	SnapshotSHA256          []byte
	SpecSHA256              []byte
	PolicySHA256            []byte
	BaselineExecutorVersion string
	TamozExecutorVersion    string
	BaselineManifestSHA256  []byte
	TamozManifestSHA256     []byte
	BaselineDecisionJSON    []byte
	BaselineDecisionSHA256  []byte
	TamozDecisionJSON       []byte
	TamozDecisionSHA256     []byte
	ComparisonJSON          []byte
	ComparisonSHA256        []byte
	CreatedAt               string
}

// ShadowComparisonStore persists paired shadow evidence without entering
// intents, commands, or outbox. Callers should validate all contract fields
// before calling Record.
type ShadowComparisonStore struct{}

// Record appends one comparison. The unique comparison key prevents a replay
// retry from overwriting an earlier artifact.
func (ShadowComparisonStore) Record(ctx context.Context, tx *sql.Tx, comparison ShadowComparison) error {
	if _, err := tx.ExecContext(ctx, insertShadowComparisonSQL, comparison.columnValues()...); err != nil {
		return fmt.Errorf("record shadow comparison: %w", err)
	}
	return nil
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
func (c ShadowComparison) columnValues() []any {
	return []any{
		c.ComparisonID, c.ComparisonKey, c.TenantID, c.EpisodeID, c.SituationID,
		c.SituationVersion, c.TriggerID, c.SnapshotSHA256, c.SpecSHA256, c.PolicySHA256,
		c.BaselineExecutorVersion, c.TamozExecutorVersion,
		c.BaselineManifestSHA256, c.TamozManifestSHA256,
		c.BaselineDecisionJSON, c.BaselineDecisionSHA256,
		c.TamozDecisionJSON, c.TamozDecisionSHA256,
		c.ComparisonJSON, c.ComparisonSHA256, c.CreatedAt,
	}
}
