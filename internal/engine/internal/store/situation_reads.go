package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Reader reads Situations from a database without a transaction.
type Reader struct{ db *storage.DB }

// NewReader binds a Situation reader to an open database.
func NewReader(db *storage.DB) Reader { return Reader{db: db} }

const listSituationsSQL = `
	SELECT situation_id, deployment_id, situation_type, entity_type, entity_id, occurrence_id, current_version,
		last_material_version, phase, status, first_event_time, latest_event_time
	FROM situations
	WHERE tenant_id = ? AND (? = '' OR entity_id = ?)
	ORDER BY latest_event_time DESC, situation_id`

// ListSituations reads the tenant's Situations, newest evidence first,
// optionally for one entity.
func (r Reader) ListSituations(ctx context.Context, tenantID, entityID string) ([]domain.SituationSummary, error) {
	rows, err := r.db.QueryContext(ctx, listSituationsSQL, tenantID, entityID, entityID)
	if err != nil {
		return nil, fmt.Errorf("list situations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var summaries []domain.SituationSummary
	for rows.Next() {
		var s domain.SituationSummary
		if err := rows.Scan(&s.SituationID, &s.DeploymentID, &s.SituationType, &s.EntityType, &s.EntityID, &s.OccurrenceID, &s.CurrentVersion, &s.LastMaterialVersion, &s.Phase, &s.Status, &s.FirstEventTime, &s.LatestEventTime); err != nil {
			return nil, fmt.Errorf("scan situation: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}

const situationVersionSQL = `
	SELECT v.situation_id, s.deployment_id, v.version, v.previous_version, v.phase, COALESCE(v.previous_phase, ''), v.severity,
		v.confidence, v.completeness, v.event_horizon, COALESCE(v.watermark, ''), v.valid_from, COALESCE(v.valid_until, ''),
		v.snapshot_sha256, v.snapshot_json, v.lineage_id, l.references_json, v.created_at
	FROM situation_versions v
	JOIN situations s ON s.situation_id = v.situation_id
	JOIN lineage_sets l ON l.lineage_id = v.lineage_id
	WHERE s.tenant_id = ? AND v.situation_id = ? AND v.version = CASE WHEN ? > 0 THEN ? ELSE s.current_version END`

// SituationVersion reads one version of a tenant's Situation with its
// evidence set; version 0 means the current version.
func (r Reader) SituationVersion(ctx context.Context, tenantID, situationID string, version int) (domain.SituationVersionRecord, error) {
	var record domain.SituationVersionRecord
	var previous sql.NullInt64
	var digest, references []byte
	err := r.db.QueryRowContext(ctx, situationVersionSQL, tenantID, situationID, version, version).Scan(&record.SituationID, &record.DeploymentID, &record.Version, &previous, &record.Phase, &record.PreviousPhase, &record.Severity,
		&record.Confidence, &record.Completeness, &record.EventHorizon, &record.Watermark, &record.ValidFrom, &record.ValidUntil,
		&digest, &record.Snapshot, &record.LineageID, &references, &record.CreatedAt)
	if err != nil {
		return domain.SituationVersionRecord{}, fmt.Errorf("read situation %s version %d: %w", situationID, version, err)
	}
	return withVersionDetails(record, previous, digest, references)
}

func withVersionDetails(record domain.SituationVersionRecord, previous sql.NullInt64, digest, references []byte) (domain.SituationVersionRecord, error) {
	if previous.Valid {
		value := int(previous.Int64)
		record.PreviousVersion = &value
	}
	record.SnapshotSHA256 = "sha256:" + hex.EncodeToString(digest)
	if err := json.Unmarshal(references, &record.Evidence); err != nil {
		return domain.SituationVersionRecord{}, fmt.Errorf("decode lineage %s: %w", record.LineageID, err)
	}
	return record, nil
}
