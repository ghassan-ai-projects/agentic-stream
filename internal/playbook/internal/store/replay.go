package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const replayVersionsSQL = `
	SELECT sv.snapshot_json, (
		SELECT d.raw_json FROM episodes e JOIN decisions d ON d.episode_id = e.episode_id
		WHERE e.situation_id = sv.situation_id AND e.situation_version = sv.version AND d.validation_status = 'accepted'
			AND NOT EXISTS (SELECT 1 FROM rule_fires f WHERE f.episode_id = e.episode_id)
		ORDER BY d.created_at LIMIT 1)
	FROM situation_versions sv JOIN situations s ON s.situation_id = sv.situation_id
	WHERE s.tenant_id = ? AND s.situation_type = ?
	ORDER BY sv.created_at, sv.situation_id, sv.version LIMIT ?`

func (t *Tx) ReplayVersions(ctx context.Context, tenantID, situationType string, limit int) ([]domain.ReplayVersion, error) {
	versions, err := storage.QueryAll(ctx, t.tx, "replay versions", scanReplayVersion, replayVersionsSQL, tenantID, situationType, limit)
	if err != nil {
		return nil, fmt.Errorf("read the situation history to replay: %w", err)
	}
	return versions, nil
}

func scanReplayVersion(rows *sql.Rows) (domain.ReplayVersion, error) {
	var snapshot, decision []byte
	if err := rows.Scan(&snapshot, &decision); err != nil {
		return domain.ReplayVersion{}, fmt.Errorf("scan replay version: %w", err)
	}
	version, err := replayedSnapshot(snapshot)
	if err != nil || decision == nil {
		return version, err
	}
	if err := json.Unmarshal(decision, &version.Decision); err != nil {
		return domain.ReplayVersion{}, fmt.Errorf("decode a replayed decision: %w", err)
	}
	return version, nil
}

func replayedSnapshot(snapshot []byte) (domain.ReplayVersion, error) {
	facts, err := decodeSnapshot(snapshot)
	if err != nil {
		return domain.ReplayVersion{}, fmt.Errorf("a replayed version: %w", err)
	}
	return domain.ReplayVersion{Facts: facts.Facts, Evidence: facts.Evidence}, nil
}

type snapshotFacts struct {
	Facts    map[string]any `json:"facts"`
	Evidence []string       `json:"evidence"`
}

func decodeSnapshot(snapshot []byte) (snapshotFacts, error) {
	var facts snapshotFacts
	if err := json.Unmarshal(snapshot, &facts); err != nil {
		return snapshotFacts{}, fmt.Errorf("decode the snapshot: %w", err)
	}
	return facts, nil
}

func (t *Tx) ShadowTallies(ctx context.Context, rule domain.RuleKey) (int, int, error) {
	var agree, disagree int
	if err := t.tx.QueryRowContext(ctx, `SELECT
			COALESCE(SUM(CASE WHEN verdict = 'agree' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN verdict = 'disagree' THEN 1 ELSE 0 END), 0)
		FROM rule_evaluations WHERE rule_id = ? AND rule_version = ? AND kind = 'shadow'`, rule.ID, rule.Version).Scan(&agree, &disagree); err != nil {
		return 0, 0, fmt.Errorf("tally the shadow evaluations of rule %s: %w", rule.ID, err)
	}
	return agree, disagree, nil
}
