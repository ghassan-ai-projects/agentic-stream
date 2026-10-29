package engine

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
)

// ResolvedAfter reads, in the caller's transaction, the event horizon of the
// first resolved version of a Situation after the given version; false when
// the occurrence has not closed.
func ResolvedAfter(ctx context.Context, tx *sql.Tx, situationID string, version int) (time.Time, bool, error) {
	return app.ResolvedAfter(ctx, store.JoinOccurrences(tx), situationID, version)
}

// ReopenedAfter reads, in the caller's transaction, the earliest event horizon
// after the given time at which a Situation of the type was open on the
// entity; false when none was.
func ReopenedAfter(ctx context.Context, tx *sql.Tx, tenantID, entityID, situationType string, after time.Time) (time.Time, bool, error) {
	return app.ReopenedAfter(ctx, store.JoinOccurrences(tx), tenantID, entityID, situationType, after)
}
