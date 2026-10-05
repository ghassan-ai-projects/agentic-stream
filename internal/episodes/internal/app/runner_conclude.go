package app

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// recordExecution persists the result of one executed attempt: a failed or
// mismatched execution fails the attempt, and a usable outcome concludes it.
func (r *Runner) recordExecution(ctx context.Context, claim *episodeClaim, outcome *Outcome, executionErr error, deadlineExceeded bool) error {
	identity := claim.identity
	switch {
	case executionErr != nil:
		return r.failAttemptStatus(ctx, identity, domain.ExecutionFailureStatus(executionErr), domain.ExecutionFailureReason(executionErr))
	case outcome == nil:
		return r.failAttemptStatus(ctx, identity, episodeledger.AttemptFailed, "executor_returned_nil_outcome")
	case outcome.AttemptID != identity.AttemptID || outcome.Fence != identity.Fence:
		return r.rejectOutcomeIdentity(ctx, identity, outcome)
	case deadlineExceeded:
		// P8 (freshness): a decision that arrives after the episode's
		// wall_time deadline is refused. The deadline is never extended to let
		// a slow model pass, and an in-process executor cannot bypass it.
		return r.failAttemptStatus(ctx, identity, episodeledger.AttemptTimedOut, "decision_after_deadline")
	default:
		return r.concludeAttempt(ctx, claim, outcome)
	}
}

func (r *Runner) rejectOutcomeIdentity(ctx context.Context, identity episodeledger.Identity, outcome *Outcome) error {

	incoming := episodeledger.Identity{EpisodeID: identity.EpisodeID, AttemptID: outcome.AttemptID, Fence: outcome.Fence}
	return r.failAttemptWithRejection(ctx, identity, incoming, domain.OutcomeIdentityRejection(identity, incoming), "worker_identity_mismatch")
}

// concludeAttempt persists a usable outcome in one transaction: the proposed
// Decision, its governance or rejection, and the attempt and episode terminals.
func (r *Runner) concludeAttempt(ctx context.Context, claim *episodeClaim, outcome *Outcome) error {
	return r.withTx(ctx, func(tx *store.Tx) error {
		now := r.runtimeNow()
		// P8 (kill, post-execute): the recorded epoch is re-asserted INSIDE
		// the persistence transaction. An attempt dispatched before a kill
		// that completes after it is refused here, so a hostile worker cannot
		// slip a decision into governance.
		reason, err := r.epochRefusal(ctx, tx, claim.req.PolicyEpoch)
		if err != nil {
			return fmt.Errorf("check post-execute policy epoch: %w", err)
		}
		if reason != "" {
			return r.abandonAfterExecute(ctx, tx, claim, outcome, reason+"_post_execute", now)
		}
		record, err := r.persistDecision(ctx, tx, claim, outcome, now)
		if err != nil {
			return err
		}
		return r.finishAttempt(ctx, tx, claim, outcome, record, now)
	})
}

// abandonAfterExecute quarantines an episode whose epoch was killed or
// unbound while its attempt ran.
func (r *Runner) abandonAfterExecute(ctx context.Context, tx *store.Tx, claim *episodeClaim, outcome *Outcome, reason string, now string) error {
	attemptTerminal, err := json.Marshal(map[string]any{
		"status": string(episodeledger.AttemptAbandoned),
		"reason": reason,
	})
	if err != nil {
		return fmt.Errorf("marshal post-execute attempt terminal: %w", err)
	}
	if err := tx.TransitionAttempt(ctx, claim.identity, episodeledger.AttemptAbandoned, r.clk.Now(), attemptTerminal); err != nil {
		return fmt.Errorf("abandon killed epoch attempt: %w", err)
	}
	if err := r.abandonEpisode(ctx, tx, claim.episodeID, map[string]any{"reason": reason}, now); err != nil {
		return fmt.Errorf("quarantine post-execute killed-epoch episode: %w", err)
	}
	return r.settleAbandonedCost(ctx, tx, claim.episodeID, outcome.CostMicrounits, now)
}

func (r *Runner) settleAbandonedCost(ctx context.Context, tx *store.Tx, episodeID string, cost uint64, now string) error {
	if r.cost != nil {
		if err := tx.SettleCost(ctx, r.cost, episodeID, cost, now); err != nil {
			return fmt.Errorf("settle post-execute quarantined episode cost: %w", err)
		}
	}
	return nil
}

// finishAttempt records the attempt terminal, settles cost, and concludes the
// episode.
func (r *Runner) finishAttempt(ctx context.Context, tx *store.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) error {
	attemptStatus := domain.TerminalAttemptStatus(outcome, record != nil, record != nil && record.ValidationErr == nil)
	if !episodeledger.IsTerminalAttempt(attemptStatus) {
		return fmt.Errorf("executor returned non-terminal attempt status %q", attemptStatus)
	}
	terminalJSON, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("marshal outcome: %w", err)
	}
	if err := tx.TransitionAttempt(ctx, claim.identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
		return fmt.Errorf("finish episode attempt: %w", err)
	}
	return r.concludeSettledEpisode(ctx, tx, claim, outcome, now, terminalJSON)
}

func (r *Runner) concludeSettledEpisode(ctx context.Context, tx *store.Tx, claim *episodeClaim, outcome *Outcome, now string, terminalJSON []byte) error {
	if r.cost != nil {
		if err := tx.SettleCost(ctx, r.cost, claim.identity.EpisodeID, outcome.CostMicrounits, now); err != nil {
			return fmt.Errorf("settle episode cost: %w", err)
		}
	}
	if err := store.ConcludeEpisode(ctx, tx, claim.episodeID, now, terminalJSON); err != nil {
		return err
	}
	return nil
}
