package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func (e *Engine) saveSituationVersion(ctx context.Context, tx *sql.Tx, partitionID int, version situations.Version) error {
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)
	write, err := newSituationWrite(version)
	if err != nil {
		return err
	}
	lineageID, err := e.recordLineage(ctx, tx, version.Evidence, now)
	if err != nil {
		return err
	}
	if err := e.upsertSituation(ctx, tx, partitionID, version, write, now); err != nil {
		return err
	}
	return e.insertSituationVersion(ctx, tx, version, lineageID, now)
}

// situationWrite is the current-row content derived from a version.
type situationWrite struct {
	occurrenceID           string
	firstEventTime         time.Time
	stateJSON, stateDigest []byte
}

// newSituationWrite requires the version's runtime state and defaults a
// missing occurrence ID and first event time.
func newSituationWrite(version situations.Version) (situationWrite, error) {
	if len(version.StateJSON) == 0 {
		return situationWrite{}, fmt.Errorf("situation %s version %d has no runtime state", version.SituationID, version.Version)
	}
	stateDigest, err := situationStateDigest(version.StateJSON, version.StateSHA256)
	if err != nil {
		return situationWrite{}, err
	}
	digest, err := canonicaljson.DecodeDigest(stateDigest)
	if err != nil {
		return situationWrite{}, fmt.Errorf("invalid situation state digest: %w", err)
	}
	return situationWrite{occurrenceID: occurrenceIDOf(version), firstEventTime: firstEventTimeOf(version), stateJSON: version.StateJSON, stateDigest: digest}, nil
}

func occurrenceIDOf(version situations.Version) string {
	if version.OccurrenceID == "" {
		return "occ-" + version.SituationID
	}
	return version.OccurrenceID
}

func firstEventTimeOf(version situations.Version) time.Time {
	if version.FirstEventTime.IsZero() {
		return version.EventHorizon
	}
	return version.FirstEventTime
}

// recordLineage stores the evidence references once per lineage ID.
func (e *Engine) recordLineage(ctx context.Context, tx *sql.Tx, evidence []string, now string) (string, error) {
	lineageID := e.lineageID(evidence)
	referencesJSON, err := json.Marshal(evidence)
	if err != nil {
		return "", fmt.Errorf("marshal evidence: %w", err)
	}
	lineageDigest := sha256.Sum256(referencesJSON)
	return lineageID, e.insertLineageSet(ctx, tx, lineageID, lineageDigest[:], referencesJSON, now)
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

func (e *Engine) upsertSituation(ctx context.Context, tx *sql.Tx, partitionID int, version situations.Version, write situationWrite, now string) error {
	if _, err := tx.ExecContext(ctx, upsertSituationSQL,
		version.SituationID, e.tenantID, e.deploymentID, version.Type, version.EntityType,
		version.EntityID, partitionID, write.occurrenceID, version.Version, version.Phase,
		"active", write.firstEventTime.Format(time.RFC3339Nano), version.EventHorizon.Format(time.RFC3339Nano),
		now, now, 1, write.stateJSON, write.stateDigest,
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
		              phase = excluded.phase,
		              status = excluded.status,
		              latest_event_time = excluded.latest_event_time,
		              updated_at = excluded.updated_at,
		              state_codec_version = excluded.state_codec_version,
		              state_json = excluded.state_json,
		              state_sha256 = excluded.state_sha256`

func (e *Engine) insertSituationVersion(ctx context.Context, tx *sql.Tx, version situations.Version, lineageID, now string) error {
	snapshotDigest, err := canonicaljson.DecodeDigest(version.SnapshotSHA256)
	if err != nil {
		return fmt.Errorf("invalid snapshot digest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, insertSituationVersionSQL,
		version.SituationID, version.Version, previousVersionOf(version), version.Phase, version.PreviousPhase,
		version.Severity, version.Confidence, version.Completeness,
		version.EventHorizon.Format(time.RFC3339Nano), version.Watermark.Format(time.RFC3339Nano),
		version.EventHorizon.Format(time.RFC3339Nano), version.SnapshotJSON, snapshotDigest,
		lineageID, nullableString(version.Traceparent), nullableString(version.Tracestate), now,
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

// previousVersionOf is NULL for a first version.
func previousVersionOf(version situations.Version) sql.NullInt64 {
	if version.PreviousVersion < 1 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(version.PreviousVersion), Valid: true}
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
