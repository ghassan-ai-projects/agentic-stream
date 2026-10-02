package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
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
	args := append([]any{e.deploymentID, e.tenantID, partitionID}, entityScopeArgs(entityID)...)
	rows, err := tx.QueryContext(ctx,
		`SELECT operator_id, state_key, state_blob FROM operator_state
			 WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
			   AND `+entityScopePredicate, args...)
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
// It is a constant so the statements that embed it stay fully parameterized;
// bind it with entityScopeArgs.
const entityScopePredicate = `(state_key = ? OR (length(state_key) > length(?) AND
		        substr(state_key, 1, length(?) + 1) = ? || char(31)))`

// entityScopeArgs returns the bind arguments for entityScopePredicate.
func entityScopeArgs(entityID string) []any {
	return []any{entityID, entityID, entityID, entityID}
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
	args := append([]any{e.deploymentID, e.tenantID, partitionID}, entityScopeArgs(entityID)...)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM operator_state
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND `+entityScopePredicate, args...); err != nil {
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
