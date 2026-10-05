package policy

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// NextPendingIntent returns the tenant's oldest intent awaiting evaluation.
func NextPendingIntent(ctx context.Context, db *sql.DB, tenantID string) (string, bool, error) {
	return store.NextPendingIntent(ctx, db, tenantID)
}
