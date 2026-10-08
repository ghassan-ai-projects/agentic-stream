package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// LoadOperatorState reads the operator state of one entity in a partition, or
// of the whole partition when entityID is empty.
func (tx *Tx) LoadOperatorState(ctx context.Context, partitionID int, entityID string) (*operators.PartitionState, error) {
	state := &operators.PartitionState{OperatorStates: make(map[string]map[string]*operators.OperatorStateBlob)}
	rows, scope, err := tx.operatorStateRows(ctx, partitionID, entityID)
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

func (tx *Tx) operatorStateRows(ctx context.Context, partitionID int, entityID string) (*sql.Rows, string, error) {
	if entityID == "" {
		rows, err := tx.partitionOperatorStateRows(ctx, partitionID)
		return rows, "partition ", err
	}
	args := append([]any{tx.deploymentID, tx.tenantID, partitionID}, entityScopeArgs(entityID)...)
	rows, err := tx.tx.QueryContext(ctx,
		`SELECT operator_id, state_key, state_blob FROM operator_state
			 WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
			   AND `+entityScopePredicate, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query operator state: %w", err)
	}
	return rows, "", nil
}

func (tx *Tx) partitionOperatorStateRows(ctx context.Context, partitionID int) (*sql.Rows, error) {
	rows, err := tx.tx.QueryContext(ctx, `
			SELECT operator_id, state_key, state_blob FROM operator_state
			WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?`,
		tx.deploymentID, tx.tenantID, partitionID)
	if err != nil {
		return nil, fmt.Errorf("query partition operator state: %w", err)
	}
	return rows, nil
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

func scanOperatorState(rows *sql.Rows, state *operators.PartitionState, scope string) error {
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

// SaveOperatorState replaces the entity's operator state as a whole. A nil
// state leaves storage untouched.
func (tx *Tx) SaveOperatorState(ctx context.Context, partitionID int, entityID string, state *operators.PartitionState, now time.Time) error {
	if state == nil {
		return nil
	}
	if err := tx.deleteOperatorState(ctx, partitionID, entityID); err != nil {
		return err
	}
	at := sources.FormatTime(now)
	for operatorID, states := range state.OperatorStates {
		for stateKey, blob := range states {
			if err := tx.upsertOperatorState(ctx, partitionID, operatorID, stateKey, blob, at); err != nil {
				return err
			}
		}
	}
	return nil
}

func (tx *Tx) deleteOperatorState(ctx context.Context, partitionID int, entityID string) error {
	args := append([]any{tx.deploymentID, tx.tenantID, partitionID}, entityScopeArgs(entityID)...)
	if _, err := tx.tx.ExecContext(ctx, `
		DELETE FROM operator_state
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND `+entityScopePredicate, args...); err != nil {
		return fmt.Errorf("retire prior operator state: %w", err)
	}
	return nil
}

func (tx *Tx) upsertOperatorState(ctx context.Context, partitionID int, operatorID, stateKey string, blob *operators.OperatorStateBlob, now string) error {
	stateJSON, err := json.Marshal(blob)
	if err != nil {
		return fmt.Errorf("marshal operator state: %w", err)
	}
	digest := sha256.Sum256(stateJSON)
	if _, err := tx.tx.ExecContext(ctx, insertOperatorStateSQL,
		tx.deploymentID, tx.tenantID, partitionID, operatorID, stateKey, 1, 1, stateJSON, digest[:], now,
	); err != nil {
		return fmt.Errorf("insert operator state: %w", err)
	}
	return nil
}

// insertOperatorStateSQL never conflicts: SaveOperatorState deletes the
// entity scope before re-inserting, so operator state is fully replaced per
// save, not mutated in place, and state_version is always 1. A conflict
// would mean deleteOperatorState missed a key, so it surfaces as an error
// rather than silently upserting.
const insertOperatorStateSQL = `
		INSERT INTO operator_state (
			deployment_id, tenant_id, partition_id, operator_id, state_key,
			state_version, codec_version, state_blob, state_sha256, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
