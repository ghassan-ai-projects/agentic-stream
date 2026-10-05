package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// quarantineStale abandons an episode whose re-bind budget is spent.
func (r *Runner) quarantineStale(ctx context.Context, tx *store.Tx, claim *episodeClaim, liveVersion int64) error {
	terminal := map[string]any{"reason": "stale_situation",
		"bound": claim.req.SituationVersion, "live": liveVersion, "rebind_attempts": claim.rebindCount}
	if err := r.abandonEpisode(ctx, tx, claim.episodeID, terminal, r.runtimeNow()); err != nil {
		return fmt.Errorf("quarantine stale episode: %w", err)
	}
	if r.telemetry != nil {
		r.telemetry.ObserveStaleRejection()
	}
	claim.quarantined = true
	return nil
}

// quarantineRebindFailure abandons an episode whose live snapshot failed
// validation. The reachable causes are database corruption or a validation
// bug (the engine validates at publish), so it logs loudly and counts under
// rebind_failures, distinct from the benign stale_rejections counter. The
// failed re-bind still consumes budget so the bounded path stays reachable.
func (r *Runner) quarantineRebindFailure(ctx context.Context, tx *store.Tx, claim *episodeClaim, liveVersion int64, rebindErr error) error {
	slog.ErrorContext(ctx, "episode re-bind failed: live snapshot invalid",
		"episode_id", claim.episodeID,
		"bound", claim.req.SituationVersion, "live", liveVersion,
		"error", rebindErr.Error())
	if err := r.abandonRebindFailure(ctx, tx, claim, liveVersion, rebindErr); err != nil {
		return err
	}
	if r.telemetry != nil {
		r.telemetry.ObserveRebindFailure()
	}
	claim.quarantined = true
	return nil
}

func (r *Runner) abandonRebindFailure(ctx context.Context, tx *store.Tx, claim *episodeClaim, liveVersion int64, rebindErr error) error {
	terminal, err := json.Marshal(map[string]any{"reason": "rebind_failed",
		"bound": claim.req.SituationVersion, "live": liveVersion,
		"rebind_attempts": claim.rebindCount + 1, "error": rebindErr.Error()})
	if err != nil {
		return fmt.Errorf("marshal rebind terminal: %w", err)
	}
	if err := tx.AbandonRebind(ctx, claim.episodeID, r.runtimeNow(), terminal); err != nil {
		return fmt.Errorf("quarantine rebind-failed episode: %w", err)
	}
	return nil
}

// quarantineRefusedEpoch enforces the P8 kill gate at dispatch: the episode's
// RECORDED policy epoch is checked independently of the worker, so a hostile
// worker cannot slip a decision through after a kill.
func (r *Runner) quarantineRefusedEpoch(ctx context.Context, tx *store.Tx, claim *episodeClaim) error {
	reason, err := r.epochRefusal(ctx, tx, claim.req.PolicyEpoch)
	if err != nil {
		return fmt.Errorf("check episode policy epoch: %w", err)
	}
	if reason == "" {
		return nil
	}
	return r.abandonRefusedEpoch(ctx, tx, claim, reason)
}

func (r *Runner) abandonRefusedEpoch(ctx context.Context, tx *store.Tx, claim *episodeClaim, reason string) error {
	now := r.runtimeNow()
	if err := r.abandonEpisode(ctx, tx, claim.episodeID, map[string]any{"reason": reason}, now); err != nil {
		return fmt.Errorf("quarantine killed-epoch episode: %w", err)
	}
	if r.cost != nil {
		if err := tx.SettleCost(ctx, r.cost, claim.episodeID, 0, now); err != nil {
			return fmt.Errorf("settle quarantined episode cost: %w", err)
		}
	}
	claim.quarantined = true
	return nil
}

// epochRefusal asks the configured refusal projection why the recorded
// policy epoch refuses a decision; "" means it allows one.
func (r *Runner) epochRefusal(ctx context.Context, tx *store.Tx, policyEpoch string) (string, error) {
	epochErr := tx.AssertDecisionEpoch(ctx, r.decisionEpoch, policyEpoch)
	return domain.DecisionEpochRefusal(epochErr, store.ErrEpochUnbound, store.ErrEpochKilled)
}

// abandonEpisode durably quarantines an episode with a terminal reason.
func (r *Runner) abandonEpisode(ctx context.Context, tx *store.Tx, episodeID string, terminal map[string]any, now string) error {
	terminalJSON, err := json.Marshal(terminal)
	if err != nil {
		return fmt.Errorf("marshal episode terminal: %w", err)
	}
	if err := tx.Abandon(ctx, episodeID, now, terminalJSON); err != nil {
		return fmt.Errorf("abandon episode: %w", err)
	}
	return nil
}

// runtimeNow formats the runner clock for durable timestamps.
func (r *Runner) runtimeNow() string {
	return r.clk.Now().UTC().Format(time.RFC3339Nano)
}
