package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
)

// skipUnadmittable records a scheduler item that can never be admitted as it
// stands, so it cannot block the queue. It reports false for any other error.
func (a *Admitter) skipUnadmittable(ctx context.Context, itemID string, now time.Time, refused domain.AdmissionAttempt, err error) (bool, error) {
	switch {
	case errors.Is(err, runtimecontrol.ErrCostReservationRejected):
		return true, a.skipCostRejected(ctx, itemID, now, err)
	case refused.Kind == episodeledger.KindReconsider && errors.Is(err, episodeledger.ErrLiveEpisodeConflict):
		return true, a.skipLiveReconsideration(ctx, itemID, now, refused, err)
	case errors.Is(err, domain.ErrFixtureRejected):
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
func (a *Admitter) skipLiveReconsideration(ctx context.Context, itemID string, now time.Time, refused domain.AdmissionAttempt, conflict error) error {
	if err := a.coalesceSkipped(ctx, itemID, now); err != nil {
		return fmt.Errorf("record skipped reconsideration %s: %w", itemID, err)
	}
	slog.WarnContext(ctx, "reconsideration episode admission skipped",
		"scheduler_item_id", itemID,
		"situation_id", refused.SituationID,
		"reason", "one_live_episode_per_situation",
		"error", conflict,
	)
	return nil
}

// skipFixture quarantines the misconfigured item loudly instead of leaving it
// pending forever, which would block the whole queue.
func (a *Admitter) skipFixture(ctx context.Context, itemID string, now time.Time, refused domain.AdmissionAttempt, refusal error) error {
	if err := a.coalesceSkipped(ctx, itemID, now); err != nil {
		return fmt.Errorf("record fixture-rejected scheduler item %s: %w", itemID, err)
	}
	slog.ErrorContext(ctx, "episode admission refused: fixture executor on a production route",
		"scheduler_item_id", itemID,
		"situation_id", refused.SituationID,
		"error", refusal,
	)
	return nil
}

// recordCostRejection explains the refusal and coalesces the item in one
// owner-fenced transaction.
func (a *Admitter) recordCostRejection(ctx context.Context, itemID string, now time.Time, rejection error) error {
	if err := a.cfg.Store.InAdmission(ctx, func(tx *store.AdmissionTx) error {
		if err := tx.AssertOwner(ctx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		if err := tx.RecordCostRejection(ctx, itemID, rejection); err != nil {
			return err
		}
		return tx.CoalesceCostRejected(ctx, itemID, now)
	}); err != nil {
		return fmt.Errorf("skip cost-rejected scheduler item %s: %w", itemID, err)
	}
	return nil
}

func (a *Admitter) coalesceSkipped(ctx context.Context, itemID string, now time.Time) error {
	if err := a.cfg.Store.InAdmission(ctx, func(tx *store.AdmissionTx) error {
		if err := tx.AssertOwner(ctx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		return tx.CoalesceSkipped(ctx, itemID, now)
	}); err != nil {
		return fmt.Errorf("coalesce scheduler item %s: %w", itemID, err)
	}
	return nil
}

func (a *Admitter) expireUnadmittable(ctx context.Context, expired []episodeledger.ExpiredItem, now time.Time) error {
	for _, item := range expired {
		if err := a.expireItem(ctx, item, now); err != nil {
			return fmt.Errorf("expire scheduler item %s: %w", item.SchedulerItemID, err)
		}
		slog.WarnContext(ctx, "scheduler item expired without admission",
			"scheduler_item_id", item.SchedulerItemID,
			"reason", item.Reason,
		)
	}
	return nil
}

func (a *Admitter) expireItem(ctx context.Context, item episodeledger.ExpiredItem, now time.Time) error {
	return a.cfg.Store.InAdmission(ctx, func(tx *store.AdmissionTx) error {
		if err := tx.AssertOwner(ctx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		return tx.Expire(ctx, item, now)
	})
}
