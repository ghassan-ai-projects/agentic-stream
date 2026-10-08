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

// Store is the only replay layer that speaks SQL, always against the
// session's isolated database.
type Store struct {
	DB *storage.DB
}

// New wraps the isolated session database.
func New(db *storage.DB) Store { return Store{DB: db} }

// SaveSpecDeployment registers the compiled spec in the isolated database.
func (s Store) SaveSpecDeployment(ctx context.Context, tenantID string, compiled *spec.CompiledSpec) error {
	err := spec.SaveDeployment(ctx, s.DB, tenantID, compiled)
	return err //nolint:wrapcheck // App wraps with the session operation name.
}

// EpisodeWorklist lists the executable episodes this replay produced, in
// canonical situation/version/trigger order.
func (s Store) EpisodeWorklist(ctx context.Context, tenantID string) ([]domain.ReplayEpisode, error) {
	rows, err := s.DB.QueryContext(ctx, episodeWorklistQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query replay items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectReplayEpisodes(rows)
}

func collectReplayEpisodes(rows *sql.Rows) ([]domain.ReplayEpisode, error) {
	var episodes []domain.ReplayEpisode
	for rows.Next() {
		var episode domain.ReplayEpisode
		var snapshotDigest []byte
		if err := rows.Scan(&episode.TriggerID, &episode.SituationID, &episode.SituationVersion, &episode.EpisodeID, &snapshotDigest); err != nil {
			return nil, fmt.Errorf("scan replay item: %w", err)
		}
		episode.SnapshotDigest = "sha256:" + hex.EncodeToString(snapshotDigest)
		episodes = append(episodes, episode.Keyed())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay items: %w", err)
	}
	return episodes, nil
}

// SituationVersionDigests lists the ordered version digests of one
// deployment for the canonical versions hash.
func (s Store) SituationVersionDigests(ctx context.Context, deploymentID string) ([]domain.VersionDigest, error) {
	rows, err := s.DB.QueryContext(ctx, situationVersionDigestsQuery, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectVersionDigests(rows)
}

func collectVersionDigests(rows *sql.Rows) ([]domain.VersionDigest, error) {
	var versions []domain.VersionDigest
	for rows.Next() {
		var r domain.VersionDigest
		if err := rows.Scan(&r.SituationID, &r.Version, &r.SHA256); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		versions = append(versions, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	return versions, nil
}

// RecordedSnapshotDigest loads the persisted snapshot digest of the situation
// version a recorded decision cites.
func (s Store) RecordedSnapshotDigest(ctx context.Context, situationID string, version int) ([]byte, error) {
	var snapshotDigest []byte
	if err := s.DB.QueryRowContext(ctx, `SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?`, situationID, version).Scan(&snapshotDigest); err != nil {
		return nil, fmt.Errorf("load recorded snapshot digest: %w", err)
	}
	return snapshotDigest, nil
}

// ShadowSnapshot loads the persisted snapshot document and digest of one
// situation version for a shadow trial.
func (s Store) ShadowSnapshot(ctx context.Context, episode domain.ReplayEpisode) ([]byte, []byte, error) {
	var snapshot, persistedDigest []byte
	if err := s.DB.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256 FROM situation_versions
		WHERE situation_id = ? AND version = ?`, episode.SituationID, episode.SituationVersion).Scan(&snapshot, &persistedDigest); err != nil {
		return nil, nil, fmt.Errorf("load shadow snapshot: %w", err)
	}
	return snapshot, persistedDigest, nil
}
