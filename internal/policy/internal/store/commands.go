package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ExistingCommandID distinguishes an absent command from a lookup failure.
func (tx *Tx) ExistingCommandID(ctx context.Context, intentID string) (string, error) {
	commandID, _, err := storage.QueryOptional[string](ctx, tx.tx, "SELECT command_id FROM commands WHERE intent_id = ?", intentID)
	if err != nil {
		return "", fmt.Errorf("find existing command: %w", err)
	}
	return commandID, nil
}

// StoreCommandOnce inserts once and returns the winning command identity on conflict.
func (tx *Tx) StoreCommandOnce(ctx context.Context, row domain.IntentRecord, command domain.CommandRecord, now time.Time) (domain.CommandRecord, string, error) {
	inserted, err := tx.InsertCommand(ctx, row, command, now)
	if err != nil || inserted {
		return command, "", err
	}
	commandID, err := tx.ExistingCommandID(ctx, row.IntentID)
	if err != nil {
		return domain.CommandRecord{}, "", err
	}
	if commandID == "" {
		return domain.CommandRecord{}, "", fmt.Errorf("command insert conflicted but no command exists for intent %s", row.IntentID)
	}
	return domain.CommandRecord{}, commandID, nil
}
