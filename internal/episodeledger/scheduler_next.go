package episodeledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// NextPendingSchedulerItem returns the tenant's next pending scheduler item that is due at
// now, in queue order: not_before, then creation, then identity.
func NextPendingSchedulerItem(ctx context.Context, db *sql.DB, tenantID string, now time.Time) (string, bool, error) {
	var itemID string
	err := db.QueryRowContext(ctx, `
		SELECT scheduler_item_id FROM scheduler_items
		WHERE tenant_id = ? AND status = 'pending' AND (not_before IS NULL OR not_before <= ?)
		ORDER BY not_before, created_at, scheduler_item_id LIMIT 1`,
		tenantID, now.Format(time.RFC3339Nano)).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find pending scheduler item: %w", err)
	}
	return itemID, true, nil
}
