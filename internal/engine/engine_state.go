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
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func (e *Engine) saveSituationRuntimeState(ctx context.Context, tx *sql.Tx, situation situations.Situation, stateJSON []byte, stateDigest string) error {
	digest, err := canonicaljson.DecodeDigest(stateDigest)
	if err != nil {
		return fmt.Errorf("invalid situation state digest: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE situations
		SET phase = ?, latest_event_time = ?, updated_at = ?, state_codec_version = 1,
		    state_json = ?, state_sha256 = ?
		WHERE situation_id = ? AND tenant_id = ? AND deployment_id = ? AND current_version = ?`,
		situation.Phase, situation.LatestEventTime.Format(time.RFC3339Nano), e.clock.Now().UTC().Format(time.RFC3339Nano),
		stateJSON, digest, situation.SituationID, e.tenantID, e.deploymentID, situation.Version)
	if err != nil {
		return fmt.Errorf("update situation runtime state: %w", err)
	}
	// The current_version guard detects a version race. A zero-row update means
	// the persisted current_version diverged from the in-memory version that
	// produced this state, which would silently leave state_json stale; surface
	// it instead of accepting it.
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("situation runtime state rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("update situation runtime state: no row at %s version %d (current_version diverged)", situation.SituationID, situation.Version)
	}
	return nil
}

func (e *Engine) loadOperatorStateForPartition(ctx context.Context, tx *sql.Tx, partitionID int) (*operators.PartitionState, error) {
	return e.readOperatorState(ctx, tx, partitionID, "")
}

func (e *Engine) loadOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, entityID string) (*operators.PartitionState, error) {
	return e.readOperatorState(ctx, tx, partitionID, entityID)
}

func (e *Engine) readOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, entityID string) (*operators.PartitionState, error) {
	state := &operators.PartitionState{OperatorStates: make(map[string]map[string]*operators.OperatorStateBlob)}
	rows, scope, err := e.operatorStateRows(ctx, tx, partitionID, entityID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scanOperatorState(rows, state, scope); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %soperator state: %w", scope, err)
	}
	return state, nil
}

func (e *Engine) operatorStateRows(ctx context.Context, tx *sql.Tx, partitionID int, entityID string) (*sql.Rows, string, error) {
	if entityID == "" {
		rows, err := tx.QueryContext(ctx, `
			SELECT operator_id, state_key, state_blob FROM operator_state
			WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?`,
			e.deploymentID, e.tenantID, partitionID)
		if err != nil {
			return nil, "partition ", fmt.Errorf("query partition operator state: %w", err)
		}
		return rows, "partition ", nil
	}
	predicate, scopeArgs := entityScopePredicate(entityID)
	args := append([]any{e.deploymentID, e.tenantID, partitionID}, scopeArgs...)
	rows, err := tx.QueryContext(ctx,
		`SELECT operator_id, state_key, state_blob FROM operator_state
			 WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
			   AND `+predicate, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query operator state: %w", err)
	}
	return rows, "", nil
}

// entityScopePredicate matches an entity's own operator state_key plus every
// composite key prefixed by "<entityID>" + char(31). char(31) (unit separator)
// is the composite-key delimiter used throughout operator state keys, so the
// prefix test cannot match a different entity whose ID shares this prefix. The
// read and delete paths share this one definition to prevent them diverging
// (a divergence would silently drop or resurrect operator state).
func entityScopePredicate(entityID string) (string, []any) {
	return `(state_key = ? OR (length(state_key) > length(?) AND
		        substr(state_key, 1, length(?) + 1) = ? || char(31)))`,
		[]any{entityID, entityID, entityID, entityID}
}

type operatorStateScanner interface {
	Scan(dest ...any) error
}

func scanOperatorState(rows operatorStateScanner, state *operators.PartitionState, scope string) error {
	var operatorID, stateKey string
	var stateJSON []byte
	if err := rows.Scan(&operatorID, &stateKey, &stateJSON); err != nil {
		return fmt.Errorf("scan %soperator state: %w", scope, err)
	}
	var blob operators.OperatorStateBlob
	if err := json.Unmarshal(stateJSON, &blob); err != nil {
		return fmt.Errorf("unmarshal %soperator state: %w", scope, err)
	}
	if state.OperatorStates[operatorID] == nil {
		state.OperatorStates[operatorID] = make(map[string]*operators.OperatorStateBlob)
	}
	state.OperatorStates[operatorID][stateKey] = &blob
	return nil
}

func (e *Engine) saveOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, entityID string, state *operators.PartitionState) error {
	if state == nil {
		return nil
	}
	if err := e.deleteOperatorState(ctx, tx, partitionID, entityID); err != nil {
		return err
	}
	now := e.clock.Now().UTC().Format(time.RFC3339Nano)
	for operatorID, states := range state.OperatorStates {
		for stateKey, blob := range states {
			if err := e.upsertOperatorState(ctx, tx, partitionID, operatorID, stateKey, blob, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Engine) deleteOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, entityID string) error {
	predicate, scopeArgs := entityScopePredicate(entityID)
	args := append([]any{e.deploymentID, e.tenantID, partitionID}, scopeArgs...)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM operator_state
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND `+predicate, args...); err != nil {
		return fmt.Errorf("retire prior operator state: %w", err)
	}
	return nil
}

func (e *Engine) upsertOperatorState(ctx context.Context, tx *sql.Tx, partitionID int, operatorID, stateKey string, blob *operators.OperatorStateBlob, now string) error {
	stateJSON, err := json.Marshal(blob)
	if err != nil {
		return fmt.Errorf("marshal operator state: %w", err)
	}
	digest := sha256.Sum256(stateJSON)
	// saveOperatorState deletes the entity scope before re-inserting, so this
	// INSERT never conflicts: operator state is fully replaced per save, not
	// mutated in place. state_version is therefore always 1. A conflict here
	// would mean deleteOperatorState missed a key, so let it surface as an
	// error rather than silently upserting (the old ON CONFLICT branch was
	// dead and its state_version+1 counter never fired).
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operator_state (
			deployment_id, tenant_id, partition_id, operator_id, state_key,
			state_version, codec_version, state_blob, state_sha256, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.deploymentID, e.tenantID, partitionID, operatorID, stateKey,
		1, 1, stateJSON, digest[:], now,
	); err != nil {
		return fmt.Errorf("insert operator state: %w", err)
	}
	return nil
}

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
