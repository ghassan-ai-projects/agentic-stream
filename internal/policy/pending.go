package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// NextPendingIntent returns the tenant's oldest intent still awaiting policy
// evaluation.
func NextPendingIntent(ctx context.Context, db *sql.DB, tenantID string) (string, bool, error) {
	var intentID string
	err := db.QueryRowContext(ctx, `
			SELECT intent_id FROM intents WHERE tenant_id = ? AND policy_status = 'pending'
			ORDER BY created_at, intent_id LIMIT 1`, tenantID).Scan(&intentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find pending intent: %w", err)
	}
	return intentID, true, nil
}
