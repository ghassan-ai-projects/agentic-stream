package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const situationVersionDigestsQuery = `
		SELECT situation_id, version, snapshot_sha256
		FROM situation_versions
		WHERE situation_id IN (
			SELECT situation_id FROM situations WHERE deployment_id = ?
		)
		ORDER BY situation_id, version`

const episodeWorklistQuery = `
		SELECT DISTINCT si.trigger_id, si.situation_id, si.situation_version, e.episode_id, sv.snapshot_sha256
		FROM scheduler_items si
		JOIN episodes e ON e.scheduler_item_id = si.scheduler_item_id
		JOIN situation_versions sv ON sv.situation_id = si.situation_id AND sv.version = si.situation_version
		WHERE si.tenant_id = ?
		ORDER BY si.situation_id, si.situation_version, si.trigger_id`

type Store struct {
	DB *storage.DB
}

func New(db *storage.DB) Store { return Store{DB: db} }

func (s Store) SaveSpecDeployment(ctx context.Context, tenantID string, compiled *spec.CompiledSpec) error {
	err := spec.SaveDeployment(ctx, s.DB, tenantID, compiled)
	return err //nolint:wrapcheck // App wraps with the session operation name.
}

func (s Store) EpisodeWorklist(ctx context.Context, tenantID string) ([]domain.ReplayEpisode, error) {
	episodes, err := storage.QueryAll(ctx, s.DB, "replay items", scanReplayEpisode, episodeWorklistQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list replay episodes: %w", err)
	}
	return episodes, nil
}

func scanReplayEpisode(rows *sql.Rows) (domain.ReplayEpisode, error) {
	var episode domain.ReplayEpisode
	var snapshotDigest []byte
	if err := rows.Scan(&episode.TriggerID, &episode.SituationID, &episode.SituationVersion, &episode.EpisodeID, &snapshotDigest); err != nil {
		return domain.ReplayEpisode{}, fmt.Errorf("scan replay item: %w", err)
	}
	episode.SnapshotDigest = "sha256:" + hex.EncodeToString(snapshotDigest)
	return episode.Keyed(), nil
}

func (s Store) SituationVersionDigests(ctx context.Context, deploymentID string) ([]domain.VersionDigest, error) {
	versions, err := storage.QueryAll(ctx, s.DB, "versions", scanVersionDigest, situationVersionDigestsQuery, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("list situation versions: %w", err)
	}
	return versions, nil
}

func scanVersionDigest(rows *sql.Rows) (domain.VersionDigest, error) {
	var version domain.VersionDigest
	if err := rows.Scan(&version.SituationID, &version.Version, &version.SHA256); err != nil {
		return domain.VersionDigest{}, fmt.Errorf("scan version: %w", err)
	}
	return version, nil
}

func (s Store) RecordedSnapshotDigest(ctx context.Context, situationID string, version int) ([]byte, error) {
	var snapshotDigest []byte
	if err := s.DB.QueryRowContext(ctx, `SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?`, situationID, version).Scan(&snapshotDigest); err != nil {
		return nil, fmt.Errorf("load recorded snapshot digest: %w", err)
	}
	return snapshotDigest, nil
}

func (s Store) ShadowSnapshot(ctx context.Context, episode domain.ReplayEpisode) ([]byte, []byte, error) {
	var snapshot, persistedDigest []byte
	if err := s.DB.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256 FROM situation_versions
		WHERE situation_id = ? AND version = ?`, episode.SituationID, episode.SituationVersion).Scan(&snapshot, &persistedDigest); err != nil {
		return nil, nil, fmt.Errorf("load shadow snapshot: %w", err)
	}
	return snapshot, persistedDigest, nil
}

func (s Store) ShadowRequest(ctx context.Context, episode domain.ReplayEpisode) (domain.EpisodeRequest, error) {
	var request domain.EpisodeRequest
	var snapshotDigest []byte
	if err := s.DB.QueryRowContext(ctx, `
		SELECT executor_name, executor_version, model_policy, prompt_version, snapshot_sha256, request_json
		FROM episodes WHERE episode_id = ?`, episode.EpisodeID).Scan(&request.ExecutorName, &request.ExecutorVersion, &request.ModelPolicy, &request.PromptVersion, &snapshotDigest, &request.RequestJSON); err != nil {
		return domain.EpisodeRequest{}, fmt.Errorf("load shadow episode request: %w", err)
	}
	request.SnapshotSHA256 = "sha256:" + hex.EncodeToString(snapshotDigest)
	return request, nil
}
