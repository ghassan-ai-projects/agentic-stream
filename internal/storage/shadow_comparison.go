package storage

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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO shadow_comparisons (
			comparison_id, comparison_key, tenant_id, episode_id, situation_id,
			situation_version, trigger_id, snapshot_sha256, spec_sha256, policy_sha256,
			baseline_executor_version, tamoz_executor_version,
			baseline_manifest_sha256, tamoz_manifest_sha256,
			baseline_decision_json, baseline_decision_sha256,
			tamoz_decision_json, tamoz_decision_sha256,
			comparison_json, comparison_sha256, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		comparison.ComparisonID, comparison.ComparisonKey, comparison.TenantID,
		comparison.EpisodeID, comparison.SituationID, comparison.SituationVersion,
		comparison.TriggerID, comparison.SnapshotSHA256, comparison.SpecSHA256,
		comparison.PolicySHA256, comparison.BaselineExecutorVersion,
		comparison.TamozExecutorVersion, comparison.BaselineManifestSHA256,
		comparison.TamozManifestSHA256, comparison.BaselineDecisionJSON,
		comparison.BaselineDecisionSHA256, comparison.TamozDecisionJSON,
		comparison.TamozDecisionSHA256, comparison.ComparisonJSON,
		comparison.ComparisonSHA256, comparison.CreatedAt); err != nil {
		return fmt.Errorf("record shadow comparison: %w", err)
	}
	return nil
}
