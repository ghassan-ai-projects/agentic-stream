package engine

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
)

// PruneReport counts the rows a retention pass removed.
type PruneReport = domain.PruneReport

// PruneHistory removes, inside the caller's transaction, the tenant's stream
// bookkeeping older than before that nothing still needs: applied inbox
// entries, fired and withdrawn timers, superseded Situation versions that no
// evaluation, scheduler item, episode, decision or intent references (never
// the current, last reasoned or last material version), and lineage sets no
// version cites. The event log itself is kept.
func PruneHistory(ctx context.Context, tx *sql.Tx, tenantID string, before time.Time) (PruneReport, error) {
	return app.PruneHistory(ctx, store.JoinRetention(tx, tenantID), before)
}
