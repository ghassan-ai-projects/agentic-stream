package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"time"
)

func (e *Engine) saveSituationVersion(ctx context.Context, tx *sql.Tx, partitionID int, version situations.Version) error {
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)
	firstEventTime := version.FirstEventTime
	if firstEventTime.IsZero() {
		firstEventTime = version.EventHorizon
	}
	occurrenceID := version.OccurrenceID
	if occurrenceID == "" {
		occurrenceID = "occ-" + version.SituationID
	}
	stateJSON := version.StateJSON
	if len(stateJSON) == 0 {
		return fmt.Errorf("situation %s version %d has no runtime state", version.SituationID, version.Version)
	}
	stateDigest, err := situationStateDigest(stateJSON, version.StateSHA256)
	if err != nil {
		return err
	}
	stateDigestBytes, err := canonicaljson.DecodeDigest(stateDigest)
	if err != nil {
		return fmt.Errorf("invalid situation state digest: %w", err)
	}

	lineageID := e.lineageID(version.Evidence)
	referencesJSON, err := json.Marshal(version.Evidence)
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	lineageDigest := sha256.Sum256(referencesJSON)
	if err := e.insertLineageSet(ctx, tx, lineageID, lineageDigest[:], referencesJSON, now); err != nil {
		return err
	}
	if err := e.upsertSituation(ctx, tx, partitionID, version, occurrenceID, firstEventTime, stateJSON, stateDigestBytes, now); err != nil {
		return err
	}
	return e.insertSituationVersion(ctx, tx, version, lineageID, now)
}

func situationStateDigest(stateJSON []byte, digest string) (string, error) {
	if digest != "" {
		return digest, nil
	}
	var document map[string]any
	if err := json.Unmarshal(stateJSON, &document); err != nil {
		return "", fmt.Errorf("decode situation state for digest: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		return "", fmt.Errorf("digest situation state: %w", err)
	}
	return digest, nil
}

func (e *Engine) insertLineageSet(ctx context.Context, tx *sql.Tx, lineageID string, digest, referencesJSON []byte, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lineage_sets (lineage_id, sha256, reference_count, references_json, created_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(lineage_id) DO NOTHING`,
		lineageID, digest, referencesJSON, now,
	); err != nil {
		return fmt.Errorf("insert lineage set: %w", err)
	}
	return nil
}

func (e *Engine) upsertSituation(ctx context.Context, tx *sql.Tx, partitionID int, version situations.Version, occurrenceID string, firstEventTime time.Time, stateJSON, stateDigest []byte, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version, phase,
			status, first_event_time, latest_event_time, updated_at, created_at,
			state_codec_version, state_json, state_sha256
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(situation_id)
		DO UPDATE SET current_version = excluded.current_version,
		              phase = excluded.phase,
		              status = excluded.status,
		              latest_event_time = excluded.latest_event_time,
		              updated_at = excluded.updated_at,
		              state_codec_version = excluded.state_codec_version,
		              state_json = excluded.state_json,
		              state_sha256 = excluded.state_sha256`,
		version.SituationID, e.tenantID, e.deploymentID, version.Type, version.EntityType,
		version.EntityID, partitionID, occurrenceID, version.Version, version.Phase,
		"active", firstEventTime.Format(time.RFC3339Nano), version.EventHorizon.Format(time.RFC3339Nano),
		now, now, 1, stateJSON, stateDigest,
	); err != nil {
		return fmt.Errorf("upsert situation: %w", err)
	}
	return nil
}

func (e *Engine) insertSituationVersion(ctx context.Context, tx *sql.Tx, version situations.Version, lineageID, now string) error {
	snapshotDigest, err := canonicaljson.DecodeDigest(version.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("invalid snapshot digest: %w", err)
	}
	var previousVersion sql.NullInt64
	if version.PreviousVersion >= 1 {
		previousVersion = sql.NullInt64{Int64: int64(version.PreviousVersion), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO situation_versions (
			situation_id, version, previous_version, phase, previous_phase,
			severity, confidence, completeness, event_horizon, watermark,
			valid_from, snapshot_json, snapshot_sha256, lineage_id, traceparent, tracestate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version.SituationID, version.Version, previousVersion, version.Phase, version.PreviousPhase,
		version.Severity, version.Confidence, version.Completeness,
		version.EventHorizon.Format(time.RFC3339Nano), version.Watermark.Format(time.RFC3339Nano),
		version.EventHorizon.Format(time.RFC3339Nano), version.SnapshotJSON, snapshotDigest,
		lineageID, nullableString(version.Traceparent), nullableString(version.Tracestate), now,
	); err != nil {
		return fmt.Errorf("insert situation version: %w", err)
	}
	return nil
}

// lineageID derives the stable identity of an ordered evidence set. Each event
// ID is length-prefixed before hashing so distinct evidence sets can never
// collide: event IDs are caller-supplied free text, and a plain concatenation
// would map e.g. ["ab","c"] and ["a","bc"] to the same lineage_id, letting the
// ON CONFLICT DO NOTHING insert in insertLineageSet silently attach the wrong
// references_json to a situation version (an explainability-invariant break).
func (e *Engine) lineageID(evidence []string) string {
	h := sha256.New()
	var lenBuf [8]byte
	for _, id := range evidence {
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(id)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write([]byte(id))
	}
	return "lin_" + hex.EncodeToString(h.Sum(nil))
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
