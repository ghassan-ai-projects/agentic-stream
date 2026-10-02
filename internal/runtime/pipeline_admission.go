package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"

	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func (p *Pipeline) assemblePending(ctx context.Context) (int, error) {
	count := 0
	for {
		now := p.clk.Now().UTC()
		// P8 (drain): while the epoch is draining or killed, no NEW episode is
		// admitted — in-flight episodes finish under their recorded epoch.
		// This is a SKIP, not a batch error: the loop keeps running so the
		// runner drains the admitted backlog.
		if p.epochControl != nil {
			if err := p.epochControl.AssertAdmission(ctx, p.ownerEpoch); err != nil {
				return count, nil
			}
		}
		itemID, found, err := p.nextPendingSchedulerItem(ctx, now)
		if err != nil || !found {
			return count, err
		}
		admitted, err := p.admitSchedulerItem(ctx, itemID, now)
		if err == nil {
			count++
			continue
		}
		skipped, skipErr := p.skipUnadmittable(ctx, itemID, now, admitted, err)
		if skipErr != nil {
			return count, skipErr
		}
		if !skipped {
			return count, fmt.Errorf("admit scheduler item %s: %w", itemID, err)
		}
	}
}

func (p *Pipeline) nextPendingSchedulerItem(ctx context.Context, now time.Time) (string, bool, error) {
	var itemID string
	err := p.db.QueryRowContext(ctx, `
		SELECT scheduler_item_id FROM scheduler_items
		WHERE tenant_id = ? AND status = 'pending' AND (not_before IS NULL OR not_before <= ?)
		ORDER BY not_before, created_at, scheduler_item_id LIMIT 1`,
		p.tenantID, now.Format(time.RFC3339Nano)).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find pending scheduler item: %w", err)
	}
	return itemID, true, nil
}

// admission identifies the request an admission attempt assembled, so a
// refusal can be reported against it.
type admission struct {
	kind, situationID string
}

// admitSchedulerItem assembles and persists one episode for a scheduler item,
// stamping the current policy epoch exactly once.
func (p *Pipeline) admitSchedulerItem(ctx context.Context, itemID string, now time.Time) (admission, error) {
	var admitted admission
	err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := p.assertOwnerTx(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		req, err := p.assembler.Assemble(ctx, tx, itemID, p.tenantID)
		if err != nil {
			return fmt.Errorf("assemble scheduler item: %w", err)
		}
		// P8 (mode matrix): a production pipeline rejects the `fixture`
		// executor — it exists for demos and tests only, never on a live
		// route. The policy epoch is stamped ONCE here, never rewritten.
		if !p.demoMode && req.ExecutorName == "fixture" {
			return fmt.Errorf("%w: fixture executor %s on a production route",
				ErrFixtureRejected, req.ExecutorName)
		}
		req.PolicyEpoch = p.ownerEpoch
		admitted = admission{kind: req.Kind, situationID: req.SituationID}
		return p.assembler.Persist(ctx, tx, req, now)
	})
	if err != nil {
		return admitted, fmt.Errorf("persist episode admission: %w", err)
	}
	return admitted, nil
}

// skipUnadmittable records a scheduler item that can never be admitted as it
// stands, so it cannot block the queue: a cost-control rejection, a
// reconsideration while its Situation has a live episode, or a fixture
// executor on a production route. It reports false for any other error.
func (p *Pipeline) skipUnadmittable(ctx context.Context, itemID string, now time.Time, admitted admission, err error) (bool, error) {
	switch {
	case errors.Is(err, costcontrol.ErrReservationRejected):
		if skipErr := p.skipCostRejectedSchedulerItem(ctx, itemID, now, err); skipErr != nil {
			return false, fmt.Errorf("record cost-rejected scheduler item %s: %w", itemID, skipErr)
		}
		slog.WarnContext(ctx, "episode admission skipped by cost control",
			"scheduler_item_id", itemID,
			"reason", err,
		)
	case admitted.kind == "reconsider" && errors.Is(err, episodeledger.ErrLiveEpisodeConflict):
		if skipErr := p.coalesceSkippedSchedulerItem(ctx, itemID, now); skipErr != nil {
			return false, fmt.Errorf("record skipped reconsideration %s: %w", itemID, skipErr)
		}
		slog.WarnContext(ctx, "reconsideration episode admission skipped",
			"scheduler_item_id", itemID,
			"situation_id", admitted.situationID,
			"reason", "one_live_episode_per_situation",
			"error", err,
		)
	case errors.Is(err, ErrFixtureRejected):
		// Quarantine the misconfigured item (loudly) instead of leaving it
		// pending forever, which would block the whole queue.
		if skipErr := p.coalesceSkippedSchedulerItem(ctx, itemID, now); skipErr != nil {
			return false, fmt.Errorf("record fixture-rejected scheduler item %s: %w", itemID, skipErr)
		}
		slog.ErrorContext(ctx, "episode admission refused: fixture executor on a production route",
			"scheduler_item_id", itemID,
			"situation_id", admitted.situationID,
			"error", err,
		)
	default:
		return false, nil
	}
	return true, nil
}

func (p *Pipeline) skipCostRejectedSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time, rejection error) error {
	if err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := p.assertOwnerTx(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		if err := cognition.RecordCostRejectionReason(ctx, tx, schedulerItemID, rejection); err != nil {
			return fmt.Errorf("%w", err)
		}
		if err := scheduleledger.CoalesceCostRejected(ctx, tx, schedulerItemID, now); err != nil {
			return fmt.Errorf("%w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("skip cost-rejected scheduler item %s: %w", schedulerItemID, err)
	}
	return nil
}

func (p *Pipeline) coalesceSkippedSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time) error {
	if err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := p.assertOwnerTx(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		if err := scheduleledger.CoalesceSkipped(ctx, tx, schedulerItemID, now); err != nil {
			return fmt.Errorf("%w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("coalesce scheduler item %s: %w", schedulerItemID, err)
	}
	return nil
}
