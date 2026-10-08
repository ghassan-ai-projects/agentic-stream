package control

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// CostLedger reserves configured episode cost before admission and settles the
// actual worker-reported cost at terminal persistence. It keeps no state; every
// operation runs on the caller's transaction.
type CostLedger struct{}

// Reserve atomically reserves amount for an episode. A zero max means
// unlimited; a kill switch always rejects new reservations.
func (CostLedger) Reserve(ctx context.Context, tx *sql.Tx, episodeID, tenantID string, amount uint64, now string) error {
	return app.Reserve(ctx, store.Join(tx), episodeID, tenantID, amount, now)
}

// Settle releases the reservation and records actual worker cost. If actual
// spend crosses a ceiling, the corresponding kill switch is tripped.
func (CostLedger) Settle(ctx context.Context, tx *sql.Tx, episodeID string, actual uint64, now string) error {
	return app.Settle(ctx, store.Join(tx), episodeID, actual, now)
}

// ApplyCostCeilings writes the global and tenant ceilings inside the caller's
// owner-fenced transaction.
func ApplyCostCeilings(ctx context.Context, tx *sql.Tx, ceilings CostCeilings, tenantID, now string) error {
	return app.ApplyCeilings(ctx, store.Join(tx), ceilings, tenantID, now)
}
