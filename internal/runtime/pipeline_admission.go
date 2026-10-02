package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"log/slog"
	"time"
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
	case admitted.kind == "reconsider" && errors.Is(err, episodes.ErrLiveEpisodeConflict):
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
		if err := recordCostRejectionReason(ctx, tx, schedulerItemID, rejection); err != nil {
			return err
		}
		return coalesceCostRejectedItem(ctx, tx, schedulerItemID, now)
	}); err != nil {
		return fmt.Errorf("skip cost-rejected scheduler item %s: %w", schedulerItemID, err)
	}
	return nil
}

func recordCostRejectionReason(ctx context.Context, tx *sql.Tx, schedulerItemID string, rejection error) error {
	var triggerID string
	var reasonsJSON []byte
	if err := tx.QueryRowContext(ctx, `
			SELECT trigger_id, reasons_json FROM trigger_evaluations
			WHERE trigger_id = (SELECT trigger_id FROM scheduler_items WHERE scheduler_item_id = ?)`, schedulerItemID).
		Scan(&triggerID, &reasonsJSON); err != nil {
		return fmt.Errorf("load cost-rejected trigger evaluation: %w", err)
	}
	var reasons []string
	if len(reasonsJSON) > 0 {
		if err := json.Unmarshal(reasonsJSON, &reasons); err != nil {
			return fmt.Errorf("decode trigger evaluation reasons: %w", err)
		}
	}
	reasons = append(reasons, "episode admission rejected by cost control: "+rejection.Error())
	reasonsJSON, err := json.Marshal(reasons)
	if err != nil {
		return fmt.Errorf("encode trigger evaluation reasons: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE trigger_evaluations SET reasons_json = ? WHERE trigger_id = ?",
		reasonsJSON, triggerID); err != nil {
		return fmt.Errorf("record cost rejection reason: %w", err)
	}

	return nil
}

func coalesceCostRejectedItem(ctx context.Context, tx *sql.Tx, schedulerItemID string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
		now.UTC().Format(time.RFC3339Nano), schedulerItemID)
	if err != nil {
		return fmt.Errorf("skip cost-rejected scheduler item: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cost-rejected scheduler item rows affected: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
	}
	return nil
}

func (p *Pipeline) coalesceSkippedSchedulerItem(ctx context.Context, schedulerItemID string, now time.Time) error {
	if err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := p.assertOwnerTx(ctx, tx); err != nil {
			return fmt.Errorf("assert pipeline owner: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE scheduler_items SET status = 'coalesced', updated_at = ?
			WHERE scheduler_item_id = ? AND status = 'pending'`,
			now.UTC().Format(time.RFC3339Nano), schedulerItemID,
		)
		if err != nil {
			return fmt.Errorf("coalesce scheduler item: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("coalesced scheduler item rows affected: %w", err)
		}
		if updated != 1 {
			return fmt.Errorf("scheduler item %s is no longer pending", schedulerItemID)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("coalesce scheduler item %s: %w", schedulerItemID, err)
	}
	return nil
}
