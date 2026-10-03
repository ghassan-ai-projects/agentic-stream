package cognition

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// verifiedCorrection decodes the corrected snapshot and requires it to match
// the digest persisted with its Situation version.
func verifiedCorrection(ctx context.Context, tx *sql.Tx, current situations.Version) (map[string]any, []byte, error) {
	correction, correctionDigest, err := decodeCorrection(current.SnapshotJSON)
	if err != nil {
		return nil, nil, err
	}
	decoded, err := matchPersistedSnapshotDigest(ctx, tx, current, correctionDigest)
	if err != nil {
		return nil, nil, err
	}
	return correction, decoded, nil
}

// decodeCorrection decodes and schema-validates the corrected snapshot and
// computes its digest.
func decodeCorrection(snapshotJSON []byte) (map[string]any, string, error) {
	var correction map[string]any
	if err := json.Unmarshal(snapshotJSON, &correction); err != nil {
		return nil, "", fmt.Errorf("decode correction snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, correction); err != nil {
		return nil, "", fmt.Errorf("validate correction snapshot: %w", err)
	}
	correctionDigest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, correction)
	if err != nil {
		return nil, "", fmt.Errorf("digest correction snapshot: %w", err)
	}
	return correction, correctionDigest, nil
}

// matchPersistedSnapshotDigest requires the recomputed correction digest to
// equal the one persisted with the version.
func matchPersistedSnapshotDigest(ctx context.Context, tx *sql.Tx, current situations.Version, correctionDigest string) ([]byte, error) {
	var persisted []byte
	if err := tx.QueryRowContext(ctx, "SELECT snapshot_sha256 FROM situation_versions WHERE situation_id = ? AND version = ?", current.SituationID, current.Version).Scan(&persisted); err != nil {
		return nil, fmt.Errorf("load correction snapshot digest: %w", err)
	}
	decoded, err := canonicaljson.DecodeDigest(correctionDigest)
	if err != nil || !bytes.Equal(decoded, persisted) {
		return nil, fmt.Errorf("correction snapshot digest mismatch")
	}
	return decoded, nil
}

func invalidatedCommands(ctx context.Context, tx *sql.Tx, current situations.Version) ([]invalidatedCommand, error) {
	rows, err := tx.QueryContext(ctx, selectInvalidatedCommandsSQL, current.SituationID, current.PreviousVersion)
	if err != nil {
		return nil, fmt.Errorf("find invalidated commands: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return storage.CollectRows(rows, "invalidated commands", scanInvalidatedCommand) //nolint:wrapcheck // CollectRows names the failed step.
}

// selectInvalidatedCommandsSQL finds succeeded commands of the latest
// approved intents at or before the previous version, with their latest
// outcome.
const selectInvalidatedCommandsSQL = `
		SELECT c.command_id, d.decision_id, o.outcome_id, o.ordinal, o.status, o.provider_result_json,
		       o.observed_effect_json, o.reconciliation_status, o.outcome_sha256
		FROM commands c
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		JOIN episodes e ON e.episode_id = d.episode_id
		JOIN outcomes o ON o.command_id = c.command_id
		WHERE i.situation_id = ?
		  AND i.situation_version = (
			SELECT MAX(i2.situation_version)
			FROM intents i2
			WHERE i2.situation_id = i.situation_id
			  AND i2.situation_version <= ?
			  AND i2.policy_status = 'approved'
		  )
		  AND d.validation_status = 'accepted' AND c.status = 'succeeded'
		  AND o.ordinal = (SELECT MAX(o2.ordinal) FROM outcomes o2 WHERE o2.command_id = c.command_id)
		ORDER BY c.command_id`

// priorOutcome is the invalidated command's latest outcome as reconsideration
// evidence.
func (c invalidatedCommand) priorOutcome() map[string]any {
	outcome := map[string]any{
		"status": c.outcomeStatus, "reconciliation_status": c.reconciliationStatus,
		"outcome_id": c.outcomeID, "ordinal": c.outcomeOrdinal,
		"outcome_sha256": "sha256:" + hex.EncodeToString(c.outcomeSHA),
	}
	if len(c.providerJSON) > 0 {
		outcome["provider_result"] = json.RawMessage(c.providerJSON)
	}
	if len(c.observedJSON) > 0 {
		outcome["observed_effect"] = json.RawMessage(c.observedJSON)
	}
	return outcome
}

func scanInvalidatedCommand(rows *sql.Rows) (invalidatedCommand, error) {
	var c invalidatedCommand
	if err := rows.Scan(&c.commandID, &c.decisionID, &c.outcomeID, &c.outcomeOrdinal, &c.outcomeStatus, &c.providerJSON, &c.observedJSON, &c.reconciliationStatus, &c.outcomeSHA); err != nil {
		return invalidatedCommand{}, fmt.Errorf("scan invalidated command: %w", err)
	}
	return c, nil
}
