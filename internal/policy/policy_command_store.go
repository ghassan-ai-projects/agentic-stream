package policy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func newCommand(g *Gateway, row intentRow, intent map[string]any, now time.Time) (commandDocument, error) {
	commandID := g.idGen.New(ids.PrefixCommand)
	target := domain.NormalizedTarget(row.IntentID, intent)
	idempotency := sha256.Sum256([]byte(row.TenantID + "|" + row.IntentID + "|" + row.IntentType + "|" + target))
	document := map[string]any{
		"command_id": commandID, "intent_id": row.IntentID, "tenant_id": row.TenantID,
		"effector_route": row.IntentType, "normalized_target": target,
		"idempotency_key": "sha256:" + hex.EncodeToString(idempotency[:]),
		"status":          "prepared", "not_before_mono_us": 0, "policy_digest": g.policyDigest,
		"payload": intent["parameters"], "created_at": domain.FormatTime(now),
	}
	return sealCommand(document, commandID, target, idempotency)
}

func insertCommand(ctx context.Context, tx *sql.Tx, row intentRow, command commandDocument, now time.Time) (bool, error) {
	result, err := tx.ExecContext(ctx, insertPolicyCommandSQL,
		command.ID, row.IntentID, row.TenantID, row.IntentType, command.Target,
		command.Key, command.JSON, command.SHA, domain.FormatTime(now), domain.FormatTime(now),
	)
	if err != nil {
		return false, fmt.Errorf("insert command: %w", err)
	}
	return commandInsertResult(result)
}

func removePreparedCommand(ctx context.Context, tx *sql.Tx, intentID, commandID string) error {
	result, err := tx.ExecContext(ctx, "DELETE FROM commands WHERE intent_id = ? AND command_id = ? AND status = 'pending'", intentID, commandID)
	if err != nil {
		return fmt.Errorf("remove rate-limited command: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove rate-limited command rows affected: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("remove rate-limited command affected %d rows", count)
	}
	return nil
}

func insertCommandOutbox(ctx context.Context, tx *sql.Tx, commandID string, commandJSON []byte, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox (
			kind, aggregate_id, aggregate_version, payload_json, status,
			available_at, created_at
		) VALUES ('command', ?, 1, ?, 'pending', ?, ?)
		ON CONFLICT(kind, aggregate_id, aggregate_version) DO NOTHING`,
		commandID, commandJSON, domain.FormatTime(now), domain.FormatTime(now),
	); err != nil {
		return fmt.Errorf("insert command outbox: %w", err)
	}
	return nil
}

func sealCommand(document map[string]any, commandID, target string, idempotency [sha256.Size]byte) (commandDocument, error) {
	commandJSON, err := canonicaljson.Marshal(document)
	if err != nil {
		return commandDocument{}, fmt.Errorf("canonicalize command: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainCommand, document)
	if err != nil {
		return commandDocument{}, fmt.Errorf("digest command: %w", err)
	}
	commandSHA, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return commandDocument{}, fmt.Errorf("decode command digest: %w", err)
	}
	return commandDocument{ID: commandID, JSON: commandJSON, SHA: commandSHA, Key: idempotency[:], Target: target}, nil
}

func commandInsertResult(result sql.Result) (bool, error) {
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("insert command rows affected: %w", err)
	}
	switch count {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("insert command affected %d rows", count)
	}
}

const insertPolicyCommandSQL = `
		INSERT INTO commands (
			command_id, intent_id, tenant_id, effector_route, normalized_target,
			idempotency_key, command_json, command_sha256, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(intent_id) DO NOTHING`
