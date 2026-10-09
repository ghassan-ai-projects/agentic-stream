package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func (tx *Tx) RecordLineage(ctx context.Context, lineage domain.Lineage, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(lineage_id) DO NOTHING`,
		lineage.ID, lineage.Digest, lineage.ReferencesJSON, kernel.FormatTime(now),
	); err != nil {
		return fmt.Errorf("insert lineage set: %w", err)
	}
	return nil
}

func (tx *Tx) UpsertSituation(ctx context.Context, partitionID int, version situations.Version, write domain.SituationWrite, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, "DELETE FROM unopened_situations WHERE situation_id = ?", version.SituationID); err != nil {
		return fmt.Errorf("forget unopened situation: %w", err)
	}
	at := kernel.FormatTime(now)
	if _, err := tx.tx.ExecContext(ctx, upsertSituationSQL,
		version.SituationID, tx.tenantID, tx.deploymentID, version.Type, version.EntityType,
		version.EntityID, partitionID, write.OccurrenceID, version.Version, version.Phase,
		"active", kernel.FormatTime(write.FirstEventTime), kernel.FormatTime(version.EventHorizon),
		at, at, 1, write.StateJSON, write.StateDigest,
	); err != nil {
		return fmt.Errorf("upsert situation: %w", err)
	}
	return nil
}

const upsertSituationSQL = `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version, phase,
			status, first_event_time, latest_event_time, updated_at, created_at,
			state_codec_version, state_json, state_sha256
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(situation_id)
		DO UPDATE SET current_version = excluded.current_version,
		              occurrence_id = excluded.occurrence_id,
		              first_event_time = excluded.first_event_time,
		              phase = excluded.phase,
		              status = excluded.status,
		              latest_event_time = excluded.latest_event_time,
		              updated_at = excluded.updated_at,
		              state_codec_version = excluded.state_codec_version,
		              state_json = excluded.state_json,
		              state_sha256 = excluded.state_sha256`

func (tx *Tx) InsertSituationVersion(ctx context.Context, version situations.Version, lineageID string, now time.Time) error {
	snapshotDigest, err := domain.DecodeSnapshotDigest(version.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("snapshot digest of %s v%d: %w", version.SituationID, version.Version, err)
	}
	if _, err := tx.tx.ExecContext(ctx, insertSituationVersionSQL,
		version.SituationID, version.Version, previousVersion(version), version.Phase, version.PreviousPhase,
		version.Severity, version.Confidence, version.Completeness,
		kernel.FormatTime(version.EventHorizon), kernel.FormatTime(version.Watermark),
		kernel.FormatTime(version.EventHorizon), version.SnapshotJSON, snapshotDigest,
		lineageID, storage.NullIfEmpty(version.Traceparent), storage.NullIfEmpty(version.Tracestate), kernel.FormatTime(now),
	); err != nil {
		return fmt.Errorf("insert situation version: %w", err)
	}
	return nil
}

const insertSituationVersionSQL = `
		INSERT INTO situation_versions (
			situation_id, version, previous_version, phase, previous_phase,
			severity, confidence, completeness, event_horizon, watermark,
			valid_from, snapshot_json, snapshot_sha256, lineage_id, traceparent, tracestate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func previousVersion(version situations.Version) sql.NullInt64 {
	previous, ok := domain.PreviousVersion(version)
	return sql.NullInt64{Int64: int64(previous), Valid: ok}
}

func (tx *Tx) SaveSituationRuntimeState(ctx context.Context, situation situations.Situation, stateJSON, stateDigest []byte, now time.Time) error {
	result, err := tx.tx.ExecContext(ctx, `
		UPDATE situations
		SET phase = ?, latest_event_time = ?, updated_at = ?, state_codec_version = 1,
		    state_json = ?, state_sha256 = ?
		WHERE situation_id = ? AND tenant_id = ? AND deployment_id = ? AND current_version = ?`,
		situation.Phase, kernel.FormatTime(situation.LatestEventTime), kernel.FormatTime(now),
		stateJSON, stateDigest, situation.SituationID, tx.tenantID, tx.deploymentID, situation.Version)
	if err != nil {
		return fmt.Errorf("update situation runtime state: %w", err)
	}
	return requireCurrentVersion(result, situation)
}

func requireCurrentVersion(result sql.Result, situation situations.Situation) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("situation runtime state rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("update situation runtime state: no row at %s version %d (current_version diverged)", situation.SituationID, situation.Version)
	}
	return nil
}

func (tx *Tx) SaveUnopenedSituationState(ctx context.Context, situation situations.Situation, stateJSON, stateDigest []byte, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, upsertUnopenedSituationSQL,
		situation.SituationID, tx.tenantID, tx.deploymentID, situation.Type, situation.EntityType, situation.EntityID,
		situation.PartitionID, situation.OccurrenceID, situation.Phase, situation.Severity, situation.Confidence,
		situation.Completeness, kernel.FormatTime(situation.FirstEventTime), kernel.FormatTime(situation.LatestEventTime),
		storage.NullIfEmpty(situation.Traceparent), storage.NullIfEmpty(situation.Tracestate),
		stateJSON, stateDigest, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("save unopened situation state: %w", err)
	}
	return nil
}

const upsertUnopenedSituationSQL = `
		INSERT INTO unopened_situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type, entity_id,
			partition_id, occurrence_id, phase, severity, confidence,
			completeness, first_event_time, latest_event_time,
			traceparent, tracestate, state_json, state_sha256, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(situation_id) DO UPDATE SET
			completeness = excluded.completeness,
			latest_event_time = excluded.latest_event_time,
			traceparent = excluded.traceparent,
			tracestate = excluded.tracestate,
			state_json = excluded.state_json,
			state_sha256 = excluded.state_sha256,
			updated_at = excluded.updated_at`
