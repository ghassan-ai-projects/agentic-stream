package policy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

func (g *Gateway) approveAutomatic(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, now time.Time) (Result, error) {
	if err := g.assertInterlock(ctx, tx, row, intent); err != nil {
		if !errors.Is(err, interlock.ErrTripped) {
			return result, err
		}
		return g.finishWithAuditReason(ctx, tx, row, result, "denied", "interlock_not_ready", interlockDenialAuditReason(err), now)
	}
	if commandID, err := existingCommandID(ctx, tx, row.IntentID); err != nil {
		return result, err
	} else if commandID != "" {
		result.Result, result.Reason, result.CommandID = "approved", "already_commanded", commandID
		return g.audit(ctx, tx, row, result, "approved", result.Reason, now)
	}

	command, err := newCommand(g, row, intent, now)
	if err != nil {
		return result, err
	}
	inserted, err := insertCommand(ctx, tx, row, command, now)
	if err != nil {
		return result, err
	}
	if !inserted {
		commandID, err := existingCommandID(ctx, tx, row.IntentID)
		if err != nil {
			return result, err
		}
		if commandID == "" {
			return result, fmt.Errorf("command insert conflicted but no command exists for intent %s", row.IntentID)
		}
		result.Result, result.Reason, result.CommandID = "approved", "already_commanded", commandID
		return g.audit(ctx, tx, row, result, "approved", result.Reason, now)
	}
	if row.RateLimitPerHour > 0 {
		overLimit, err := g.dispatchWithinLimit(ctx, tx, row, now)
		if err != nil {
			return result, err
		}
		if overLimit {
			if err := removePreparedCommand(ctx, tx, row.IntentID, command.ID); err != nil {
				return result, err
			}
			return g.finish(ctx, tx, row, result, "denied", "rate_limited", now)
		}
	}
	commandID := command.ID
	if err := insertCommandOutbox(ctx, tx, commandID, command.JSON, now); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'approved', updated_at = ? WHERE intent_id = ?", formatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("approve intent: %w", err)
	}
	if result.Reason == "" {
		result.Reason = "automatic_r0_r1"
	}
	result.Result, result.CommandID = "approved", commandID
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func interlockDenialAuditReason(err error) string {
	const prefix = "assert action interlock: "
	detail := strings.TrimSpace(strings.TrimPrefix(err.Error(), prefix))
	if detail == "" {
		return "interlock_not_ready"
	}
	return "interlock_not_ready: " + detail
}

func (g *Gateway) assertInterlock(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any) error {
	if g.interlock == nil {
		return nil
	}
	if err := g.interlock.Assert(ctx, tx, row.TenantID, normalizedTarget(row.IntentID, intent), row.RiskClass); err != nil {
		return fmt.Errorf("assert action interlock: %w", err)
	}
	return nil
}

// existingCommandID returns an empty ID when the intent has no command. All
// other lookup failures are returned so callers cannot confuse missing data
// with a storage failure.
func existingCommandID(ctx context.Context, tx *sql.Tx, intentID string) (string, error) {
	var commandID string
	err := tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", intentID).Scan(&commandID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find existing command: %w", err)
	}
	return commandID, nil
}

type commandDocument struct {
	ID     string
	JSON   []byte
	SHA    []byte
	Key    []byte
	Target string
}

func newCommand(g *Gateway, row intentRow, intent map[string]any, now time.Time) (commandDocument, error) {
	commandID := g.idGen.New(ids.PrefixCommand)
	target := normalizedTarget(row.IntentID, intent)
	idempotency := sha256.Sum256([]byte(row.TenantID + "|" + row.IntentID + "|" + row.IntentType + "|" + target))
	document := map[string]any{
		"command_id": commandID, "intent_id": row.IntentID, "tenant_id": row.TenantID,
		"effector_route": row.IntentType, "normalized_target": target,
		"idempotency_key": "sha256:" + hex.EncodeToString(idempotency[:]),
		"status":          "prepared", "not_before_mono_us": 0, "policy_digest": g.policyDigest,
		"payload": intent["parameters"], "created_at": formatTime(now),
	}
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

func insertCommand(ctx context.Context, tx *sql.Tx, row intentRow, command commandDocument, now time.Time) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO commands (
			command_id, intent_id, tenant_id, effector_route, normalized_target,
			idempotency_key, command_json, command_sha256, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(intent_id) DO NOTHING`,
		command.ID, row.IntentID, row.TenantID, row.IntentType, command.Target,
		command.Key, command.JSON, command.SHA, formatTime(now), formatTime(now),
	)
	if err != nil {
		return false, fmt.Errorf("insert command: %w", err)
	}
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
		commandID, commandJSON, formatTime(now), formatTime(now),
	); err != nil {
		return fmt.Errorf("insert command outbox: %w", err)
	}
	return nil
}
