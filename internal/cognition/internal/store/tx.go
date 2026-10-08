package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

type Tx struct{ tx *sql.Tx }

func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

func (t *Tx) Configured() bool { return t != nil && t.tx != nil }

func (t *Tx) InsertItem(ctx context.Context, item episodeledger.SchedulerItem, tenantID string, key []byte, now time.Time) error {
	return episodeledger.UpsertSchedulerItem(ctx, t.tx, item, tenantID, key, now) //nolint:wrapcheck // Preserve the owning ledger error contract.
}

// WithdrawSuperseded withdraws the approvals a newer Situation version
// superseded and publishes each withdrawal in this transaction, reading the
// clock after each withdrawal.
func (t *Tx) WithdrawSuperseded(ctx context.Context, situationID, tenantID string, version int, now time.Time, clk sources.Clock) error {
	publish := withdrawalPublisher(tenantID, clk)
	return approvalledger.WithdrawSuperseded(ctx, t.tx, situationID, version, kernel.FormatTime(now), publish) //nolint:wrapcheck // Preserve the owning ledger error contract.
}

// withdrawalPublisher publishes the approval.withdrawn notification of a
// superseded approval in the withdrawing transaction.
func withdrawalPublisher(tenantID string, clk sources.Clock) approvalledger.WithdrawalPublisher {
	return func(ctx context.Context, tx *sql.Tx, w approvalledger.Withdrawal) error {
		if err := notify.AppendLifecycleEvent(ctx, tx, supersededWithdrawalEvent(w, tenantID, clk)); err != nil {
			return fmt.Errorf("append superseded approval notification: %w", err)
		}
		return nil
	}
}

func supersededWithdrawalEvent(w approvalledger.Withdrawal, tenantID string, clk sources.Clock) notify.LifecycleEvent {
	payload := notify.ApprovalWithdrawn{
		ApprovalID: w.ApprovalID, IntentID: w.IntentID, SituationID: w.SituationID,
		SituationVersion: w.SituationVersion, Reason: "situation_version_conflict",
	}
	trace := contractsv1.TraceContext{Traceparent: w.Traceparent, Tracestate: w.Tracestate}
	return notify.ApprovalWithdrawnEvent(tenantID, payload, clk.Now().UTC(), trace)
}
