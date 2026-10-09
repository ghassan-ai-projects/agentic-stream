package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func NextPendingIntent(ctx context.Context, db *sql.DB, tenantID string) (string, bool, error) {
	intentID, found, err := storage.QueryOptional[string](ctx, db, nextPendingIntentSQL, tenantID)
	if err != nil {
		return "", false, fmt.Errorf("find pending intent: %w", err)
	}
	return intentID, found, nil
}

const nextPendingIntentSQL = `
			SELECT intent_id FROM intents WHERE tenant_id = ? AND policy_status = 'pending'
			ORDER BY created_at, intent_id LIMIT 1`
