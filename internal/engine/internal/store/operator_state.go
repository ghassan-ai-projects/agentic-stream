package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

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

const entityScopePredicate = `(state_key = ? OR (length(state_key) > length(?) AND
		        substr(state_key, 1, length(?) + 1) = ? || char(31)))`

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

func (tx *Tx) SaveOperatorState(ctx context.Context, partitionID int, entityID string, state *operators.PartitionState, now time.Time) error {
	if state == nil {
		return nil
	}
	stored, err := tx.storedOperatorDigests(ctx, partitionID, entityID)
	if err != nil {
		return err
	}
	encoded, err := encodeOperatorStates(state)
	if err != nil {
		return err
	}
	if err := tx.writeChangedOperatorStates(ctx, partitionID, stored, encoded, kernel.FormatTime(now)); err != nil {
		return err
	}
	return tx.deleteRetiredOperatorStates(ctx, partitionID, stored, encoded)
}

type operatorStateRef struct{ operatorID, stateKey string }

type encodedOperatorState struct {
	blob   []byte
	digest []byte
}

func (tx *Tx) storedOperatorDigests(ctx context.Context, partitionID int, entityID string) (map[operatorStateRef][]byte, error) {
	args := append([]any{tx.deploymentID, tx.tenantID, partitionID}, entityScopeArgs(entityID)...)
	rows, err := tx.tx.QueryContext(ctx, `
		SELECT operator_id, state_key, state_sha256 FROM operator_state
		WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ?
		  AND `+entityScopePredicate, args...)
	if err != nil {
		return nil, fmt.Errorf("query stored operator state digests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanOperatorDigests(rows)
}

func scanOperatorDigests(rows *sql.Rows) (map[operatorStateRef][]byte, error) {
	stored := make(map[operatorStateRef][]byte)
	for rows.Next() {
		var ref operatorStateRef
		var digest []byte
		if err := rows.Scan(&ref.operatorID, &ref.stateKey, &digest); err != nil {
			return nil, fmt.Errorf("scan stored operator state digest: %w", err)
		}
		stored[ref] = digest
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stored operator state digests: %w", err)
	}
	return stored, nil
}

func encodeOperatorStates(state *operators.PartitionState) (map[operatorStateRef]encodedOperatorState, error) {
	encoded := make(map[operatorStateRef]encodedOperatorState)
	for operatorID, states := range state.OperatorStates {
		for stateKey, blob := range states {
			stateJSON, err := json.Marshal(blob)
			if err != nil {
				return nil, fmt.Errorf("marshal operator state %s/%s: %w", operatorID, stateKey, err)
			}
			digest := sha256.Sum256(stateJSON)
			encoded[operatorStateRef{operatorID, stateKey}] = encodedOperatorState{blob: stateJSON, digest: digest[:]}
		}
	}
	return encoded, nil
}

func (tx *Tx) writeChangedOperatorStates(ctx context.Context, partitionID int, stored map[operatorStateRef][]byte, encoded map[operatorStateRef]encodedOperatorState, now string) error {
	for ref, state := range encoded {
		if bytes.Equal(stored[ref], state.digest) {
			continue
		}
		if _, err := tx.tx.ExecContext(ctx, upsertOperatorStateSQL,
			tx.deploymentID, tx.tenantID, partitionID, ref.operatorID, ref.stateKey, 1, 1, state.blob, state.digest, now,
		); err != nil {
			return fmt.Errorf("write operator state %s/%s: %w", ref.operatorID, ref.stateKey, err)
		}
	}
	return nil
}

func (tx *Tx) deleteRetiredOperatorStates(ctx context.Context, partitionID int, stored map[operatorStateRef][]byte, encoded map[operatorStateRef]encodedOperatorState) error {
	for ref := range stored {
		if _, kept := encoded[ref]; kept {
			continue
		}
		if _, err := tx.tx.ExecContext(ctx, `
			DELETE FROM operator_state
			WHERE deployment_id = ? AND tenant_id = ? AND partition_id = ? AND operator_id = ? AND state_key = ?`,
			tx.deploymentID, tx.tenantID, partitionID, ref.operatorID, ref.stateKey); err != nil {
			return fmt.Errorf("retire operator state %s/%s: %w", ref.operatorID, ref.stateKey, err)
		}
	}
	return nil
}

const upsertOperatorStateSQL = `
		INSERT INTO operator_state (
			deployment_id, tenant_id, partition_id, operator_id, state_key,
			state_version, codec_version, state_blob, state_sha256, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(deployment_id, tenant_id, partition_id, operator_id, state_key) DO UPDATE SET
			state_blob = excluded.state_blob,
			state_sha256 = excluded.state_sha256,
			updated_at = excluded.updated_at`
