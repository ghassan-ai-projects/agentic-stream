package store

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

type Tx struct{ tx *sql.Tx }

func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

func (t *Tx) Configured() bool { return t != nil && t.tx != nil }

func (t *Tx) InsertItem(ctx context.Context, item episodeledger.SchedulerItem, tenantID string, key []byte, now string) error {
	return episodeledger.UpsertSchedulerItem(ctx, t.tx, item, tenantID, key, now) //nolint:wrapcheck // Preserve the owning ledger error contract.
}

func (t *Tx) WithdrawSuperseded(ctx context.Context, situationID, tenantID string, version int, now string, clk clock.Clock) error {
	return approvalledger.WithdrawSuperseded(ctx, t.tx, situationID, tenantID, version, now, clk) //nolint:wrapcheck // Preserve the owning ledger error contract.
}
