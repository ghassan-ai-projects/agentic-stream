package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

func (p *PipelineStore) PollSchedulerQueue(ctx context.Context, now time.Time) (episodeledger.QueuePoll, error) {
	poll, err := episodeledger.PollSchedulerQueue(ctx, p.DB.DB, p.TenantID, now)
	if err != nil {
		return poll, fmt.Errorf("poll scheduler queue of tenant %s: %w", p.TenantID, err)
	}
	return poll, nil
}

// AdmissionTx is one owner-fenced admission transaction. It joins the episode
// assembler, the scheduler queue and the cost-rejection record to the
// caller-owned transaction and never begins or commits it.
type AdmissionTx struct {
	tx    *sql.Tx
	store *PipelineStore
}

// InAdmission runs one admission transaction.
func (p *PipelineStore) InAdmission(ctx context.Context, work func(*AdmissionTx) error) error {
	return p.DB.WithTx(ctx, func(tx *sql.Tx) error { return work(&AdmissionTx{tx: tx, store: p}) })
}

func (t *AdmissionTx) AssertOwner(ctx context.Context) error {
	return assertRuntimeOwner(ctx, t.tx, t.store.RuntimeOwner, t.store.OwnerEpoch)
}

// Assemble builds the scheduler item's episode request.
func (t *AdmissionTx) Assemble(ctx context.Context, itemID string) (*episodes.Request, error) {
	req, err := t.store.Episodes.Assemble(ctx, t.tx, itemID, t.store.TenantID)
	if err != nil {
		return nil, fmt.Errorf("assemble scheduler item: %w", err)
	}
	return req, nil
}

// Persist admits the assembled request.
func (t *AdmissionTx) Persist(ctx context.Context, req *episodes.Request, now time.Time) error {
	return t.store.Episodes.Persist(ctx, t.tx, req, now) //nolint:wrapcheck // The caller wraps it with the admission step.
}

// RecordCostRejection explains why cost control refused the item.
func (t *AdmissionTx) RecordCostRejection(ctx context.Context, itemID string, rejection error) error {
	return cognition.RecordCostRejectionReason(ctx, t.tx, itemID, rejection) //nolint:wrapcheck // The caller wraps it with the skip step.
}

// CoalesceCostRejected removes a cost-rejected item from the pending queue.
func (t *AdmissionTx) CoalesceCostRejected(ctx context.Context, itemID string, now time.Time) error {
	return episodeledger.CoalesceCostRejectedItem(ctx, t.tx, itemID, now) //nolint:wrapcheck // The owning ledger's error is wrapped by the caller.
}

// CoalesceSkipped removes an unavailable item from the pending queue.
func (t *AdmissionTx) CoalesceSkipped(ctx context.Context, itemID string, now time.Time) error {
	return episodeledger.CoalesceSkippedItem(ctx, t.tx, itemID, now) //nolint:wrapcheck // The owning ledger's error is wrapped by the caller.
}

func (t *AdmissionTx) Expire(ctx context.Context, expired episodeledger.ExpiredItem, now time.Time) error {
	if err := cognition.RecordSchedulerExpiryReason(ctx, t.tx, expired.SchedulerItemID, expired.Reason); err != nil {
		return fmt.Errorf("record scheduler item expiry: %w", err)
	}
	if err := episodeledger.ExpireSchedulerItem(ctx, t.tx, expired.SchedulerItemID, now); err != nil {
		return fmt.Errorf("expire scheduler item %s: %w", expired.SchedulerItemID, err)
	}
	return nil
}
