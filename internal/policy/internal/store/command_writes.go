package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"time"
)

func (tx *Tx) InsertCommand(ctx context.Context, row domain.IntentRecord, command domain.CommandRecord, now time.Time) (bool, error) {
	result, err := tx.tx.ExecContext(ctx, insertPolicyCommandSQL,
		command.ID, row.IntentID, row.TenantID, row.IntentType, command.Target,
		command.Key, command.JSON, command.SHA, domain.FormatTime(now), domain.FormatTime(now),
	)
	if err != nil {
		return false, fmt.Errorf("insert command: %w", err)
	}
	return commandInsertResult(result)
}

func (tx *Tx) RemovePreparedCommand(ctx context.Context, intentID, commandID string) error {
	result, err := tx.tx.ExecContext(ctx, "DELETE FROM commands WHERE intent_id = ? AND command_id = ? AND status = 'pending'", intentID, commandID)
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

func (tx *Tx) InsertCommandOutbox(ctx context.Context, commandID string, commandJSON []byte, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `
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
