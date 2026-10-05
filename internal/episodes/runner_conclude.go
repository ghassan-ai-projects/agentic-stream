package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"

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
	reason := episodeledger.RejectWrongAttempt
	if outcome.Fence < identity.Fence {
		reason = episodeledger.RejectStaleAttempt
	}
	incoming := episodeledger.Identity{EpisodeID: identity.EpisodeID, AttemptID: outcome.AttemptID, Fence: outcome.Fence}
	return r.failAttemptWithRejection(ctx, identity, incoming, reason, "worker_identity_mismatch")
}

// concludeAttempt persists a usable outcome in one transaction: the proposed
// Decision, its governance or rejection, and the attempt and episode terminals.
func (r *Runner) concludeAttempt(ctx context.Context, claim *episodeClaim, outcome *Outcome) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
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
func (r *Runner) abandonAfterExecute(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, reason string, now string) error {
	attemptTerminal, err := json.Marshal(map[string]any{
		"status": string(episodeledger.AttemptAbandoned),
		"reason": reason,
	})
	if err != nil {
		return fmt.Errorf("marshal post-execute attempt terminal: %w", err)
	}
	if err := episodeledger.TransitionAttempt(ctx, tx, claim.identity, episodeledger.AttemptAbandoned, r.clk.Now(), attemptTerminal); err != nil {
		return fmt.Errorf("abandon killed epoch attempt: %w", err)
	}
	if err := r.abandonEpisode(ctx, tx, claim.episodeID, map[string]any{"reason": reason}, now); err != nil {
		return fmt.Errorf("quarantine post-execute killed-epoch episode: %w", err)
	}
	return r.settleAbandonedCost(ctx, tx, claim.episodeID, outcome.CostMicrounits, now)
}

func (r *Runner) settleAbandonedCost(ctx context.Context, tx *sql.Tx, episodeID string, cost uint64, now string) error {
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, episodeID, cost, now); err != nil {
			return fmt.Errorf("settle post-execute quarantined episode cost: %w", err)
		}
	}
	return nil
}

// finishAttempt records the attempt terminal, settles cost, and concludes the
// episode.
func (r *Runner) finishAttempt(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) error {
	attemptStatus := terminalAttemptStatus(outcome, record)
	if !episodeledger.IsTerminalAttempt(attemptStatus) {
		return fmt.Errorf("executor returned non-terminal attempt status %q", attemptStatus)
	}
	terminalJSON, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("marshal outcome: %w", err)
	}
	if err := episodeledger.TransitionAttempt(ctx, tx, claim.identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
		return fmt.Errorf("finish episode attempt: %w", err)
	}
	return r.concludeSettledEpisode(ctx, tx, claim, outcome, now, terminalJSON)
}

// terminalAttemptStatus derives the attempt terminal: a Decision makes it
// produced or failed by validation; otherwise the executor's status stands,
// and an empty status means the executor declined.
func terminalAttemptStatus(outcome *Outcome, record *decisionRecord) episodeledger.AttemptStatus {
	switch {
	case record != nil && record.validationErr == nil:
		return episodeledger.AttemptProduced
	case record != nil:
		return episodeledger.AttemptFailed
	case outcome.Status == "":
		return episodeledger.AttemptDeclined
	default:
		return episodeledger.AttemptStatus(outcome.Status)
	}
}

func (r *Runner) concludeSettledEpisode(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, now string, terminalJSON []byte) error {
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, claim.identity.EpisodeID, outcome.CostMicrounits, now); err != nil {
			return fmt.Errorf("settle episode cost: %w", err)
		}
	}
	if err := episodeledger.Conclude(ctx, tx, claim.episodeID, now, terminalJSON); err != nil {
		return fmt.Errorf("update episode terminal: %w", err)
	}
	return nil
}
