package eventlog

import (
	"context"
	"database/sql"
	"time"

	app "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/app"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
)

// LatestEventTime reads, in the caller's transaction, the latest event time
// the tenant's log holds; false when the log is empty.
func LatestEventTime(ctx context.Context, tx *sql.Tx, tenantID string) (time.Time, bool, error) {
	return app.LatestEventTime(ctx, store.JoinUnit(tx), tenantID)
}
