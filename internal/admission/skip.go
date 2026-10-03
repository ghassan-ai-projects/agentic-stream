package admission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"
)

// skipUnadmittable records a scheduler item that can never be admitted as it
// stands, so it cannot block the queue. It reports false for any other error.
func (a *Admitter) skipUnadmittable(ctx context.Context, itemID string, now time.Time, refused attempt, err error) (bool, error) {
	switch {
	case errors.Is(err, costcontrol.ErrReservationRejected):
		return true, a.skipCostRejected(ctx, itemID, now, err)
	case refused.kind == "reconsider" && errors.Is(err, episodeledger.ErrLiveEpisodeConflict):
		return true, a.skipLiveReconsideration(ctx, itemID, now, refused, err)
	case errors.Is(err, ErrFixtureRejected):
		return true, a.skipFixture(ctx, itemID, now, refused, err)
	default:
		return false, nil
	}
}

func (a *Admitter) skipCostRejected(ctx context.Context, itemID string, now time.Time, rejection error) error {
	if err := a.recordCostRejection(ctx, itemID, now, rejection); err != nil {
		return fmt.Errorf("record cost-rejected scheduler item %s: %w", itemID, err)
	}
	slog.WarnContext(ctx, "episode admission skipped by cost control",
		"scheduler_item_id", itemID,
		"reason", rejection,
	)
	return nil
}

// skipLiveReconsideration enforces one live episode per Situation for
// reconsiderations.
func (a *Admitter) skipLiveReconsideration(ctx context.Context, itemID string, now time.Time, refused attempt, conflict error) error {
	if err := a.coalesceSkipped(ctx, itemID, now); err != nil {
		return fmt.Errorf("record skipped reconsideration %s: %w", itemID, err)
	}
	slog.WarnContext(ctx, "reconsideration episode admission skipped",
		"scheduler_item_id", itemID,
		"situation_id", refused.situationID,
		"reason", "one_live_episode_per_situation",
		"error", conflict,
	)
	return nil
}

// skipFixture quarantines the misconfigured item loudly instead of leaving it
// pending forever, which would block the whole queue.
func (a *Admitter) skipFixture(ctx context.Context, itemID string, now time.Time, refused attempt, refusal error) error {
	if err := a.coalesceSkipped(ctx, itemID, now); err != nil {
		return fmt.Errorf("record fixture-rejected scheduler item %s: %w", itemID, err)
	}
	slog.ErrorContext(ctx, "episode admission refused: fixture executor on a production route",
		"scheduler_item_id", itemID,
		"situation_id", refused.situationID,
		"error", refusal,
	)
	return nil
}

// recordCostRejection explains the refusal and coalesces the item in one
// owner-fenced transaction.
func (a *Admitter) recordCostRejection(ctx context.Context, itemID string, now time.Time, rejection error) error {
	if err := a.cfg.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.assertOwner(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		if err := cognition.RecordCostRejectionReason(ctx, tx, itemID, rejection); err != nil {
			return fmt.Errorf("%w", err)
		}
		return scheduleledger.CoalesceCostRejected(ctx, tx, itemID, now) //nolint:wrapcheck // The owning ledger's error is wrapped below.
	}); err != nil {
		return fmt.Errorf("skip cost-rejected scheduler item %s: %w", itemID, err)
	}
	return nil
}

func (a *Admitter) coalesceSkipped(ctx context.Context, itemID string, now time.Time) error {
	if err := a.cfg.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := a.assertOwner(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		return scheduleledger.CoalesceSkipped(ctx, tx, itemID, now) //nolint:wrapcheck // The owning ledger's error is wrapped below.
	}); err != nil {
		return fmt.Errorf("coalesce scheduler item %s: %w", itemID, err)
	}
	return nil
}
