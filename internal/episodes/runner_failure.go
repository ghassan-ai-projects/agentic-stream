package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// deadlineExceeded reports whether the attempt ran past its wall_time budget.
// The budget is the executor's canonical input (never extended after
// admission); a decision produced after it is refused.
func (r *Runner) deadlineExceeded(req *Request, startedAt time.Time) bool {
	wallTime, err := req.WallTimeBudget()
	if err != nil || wallTime <= 0 {
		return false
	}
	return r.clk.Now().After(startedAt.Add(wallTime))
}

func (r *Runner) watchSupersession(ctx context.Context, episodeID string, cancel context.CancelFunc, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var lifecycle string
			if err := r.db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err == nil && lifecycle == string(episodeledger.LifecycleSuperseded) {
				cancel()
				return
			}
		}
	}
}

func executionFailureStatus(err error) episodeledger.AttemptStatus {
	if errors.Is(err, context.Canceled) {
		return episodeledger.AttemptCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return episodeledger.AttemptTimedOut
	}
	return episodeledger.AttemptFailed
}

func executionFailureReason(err error) string {
	var budgetErr *BudgetExceededError
	if errors.As(err, &budgetErr) {
		return "budget_exhausted"
	}
	var telemetryErr BudgetTelemetryMissingError
	if errors.As(err, &telemetryErr) {
		return "budget_telemetry_missing"
	}
	switch executionFailureStatus(err) {
	case episodeledger.AttemptCancelled:
		return "worker_cancelled" //nolint:misspell // Durable protocol reason is frozen as cancelled.
	case episodeledger.AttemptTimedOut:
		return "worker_deadline_exceeded"
	default:
		return "worker_execution_failed"
	}
}

// failAttemptStatus fails the current attempt, then retries the episode or
// concludes it once the retry budget is spent.
func (r *Runner) failAttemptStatus(ctx context.Context, identity episodeledger.Identity, attemptStatus episodeledger.AttemptStatus, reason string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		terminalJSON, err := json.Marshal(map[string]any{"status": attemptStatus, "reason": reason})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if attemptStatus == episodeledger.AttemptCancelled {
			if err := r.markCancelling(ctx, tx, identity); err != nil {
				return err
			}
		}
		if err := episodeledger.TransitionAttempt(ctx, tx, identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish failed episode attempt: %w", err)
		}
		return r.retryOrConcludeFailedEpisode(ctx, tx, identity.EpisodeID)
	})
}

// markCancelling moves the attempt through episodeledger.AttemptCancelling, which the
// attempt lifecycle requires before episodeledger.AttemptCancelled.
func (r *Runner) markCancelling(ctx context.Context, tx *sql.Tx, identity episodeledger.Identity) error {
	var current episodeledger.AttemptStatus
	if err := tx.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&current); err != nil {
		return fmt.Errorf("read episode attempt status: %w", err)
	}
	if current == episodeledger.AttemptCancelling {
		return nil
	}
	if err := episodeledger.TransitionAttempt(ctx, tx, identity, episodeledger.AttemptCancelling, r.clk.Now(), nil); err != nil {
		return fmt.Errorf("mark episode attempt cancelling: %w", err) //nolint:misspell // Durable lifecycle value is frozen as cancelling.
	}
	return nil
}

// retryOrConcludeFailedEpisode keeps a failed episode running for another
// attempt, or concludes it and settles its cost when the episode was
// superseded or has used maxEpisodeAttempts.
func (r *Runner) retryOrConcludeFailedEpisode(ctx context.Context, tx *sql.Tx, episodeID string) error {
	var lifecycle string
	if err := tx.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err != nil {
		return fmt.Errorf("read episode lifecycle: %w", err)
	}
	failedAttempts, err := countFailedAttempts(ctx, tx, episodeID)
	if err != nil {
		return fmt.Errorf("count failed episode attempts: %w", err)
	}
	superseded := lifecycle == string(episodeledger.LifecycleSuperseded)
	if !superseded && failedAttempts < maxEpisodeAttempts {
		if err := episodeledger.RetainForRetry(ctx, tx, episodeID); err != nil {
			return fmt.Errorf("update episode failed: %w", err)
		}
		return nil
	}
	if !superseded {
		terminalJSON, err := json.Marshal(map[string]any{"status": episodeledger.AttemptFailed, "reason": "attempt_retry_limit"})
		if err != nil {
			return fmt.Errorf("marshal retry limit terminal: %w", err)
		}
		if err := episodeledger.Conclude(ctx, tx, episodeID, r.runtimeNow(), terminalJSON); err != nil {
			return fmt.Errorf("conclude exhausted episode: %w", err)
		}
	}
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, episodeID, 0, r.runtimeNow()); err != nil {
			return fmt.Errorf("settle abandoned episode cost: %w", err)
		}
	}
	return nil
}

// failAttemptWithRejection records why the worker's identity was rejected,
// fails the current attempt, and retries or concludes the episode.
func (r *Runner) failAttemptWithRejection(ctx context.Context, current, incoming episodeledger.Identity, reason episodeledger.RejectionReason, detail string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		details, err := json.Marshal(map[string]any{"message": detail, "incoming_attempt_id": incoming.AttemptID, "incoming_fence": incoming.Fence})
		if err != nil {
			return fmt.Errorf("marshal identity rejection: %w", err)
		}
		if err := episodeledger.RecordRejection(ctx, tx, incoming, reason, details, r.clk.Now()); err != nil {
			return fmt.Errorf("record worker identity rejection: %w", err)
		}
		terminalJSON, err := json.Marshal(map[string]any{"status": episodeledger.AttemptFailed, "reason": detail})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if err := episodeledger.TransitionAttempt(ctx, tx, current, episodeledger.AttemptFailed, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish identity-failed attempt: %w", err)
		}
		return r.retryOrConcludeRejectedEpisode(ctx, tx, current.EpisodeID, terminalJSON)
	})
}

// retryOrConcludeRejectedEpisode keeps the episode running for another
// attempt until maxEpisodeAttempts have failed, then settles its cost and
// concludes it with the rejection terminal.
func (r *Runner) retryOrConcludeRejectedEpisode(ctx context.Context, tx *sql.Tx, episodeID string, terminalJSON []byte) error {
	failedAttempts, err := countFailedAttempts(ctx, tx, episodeID)
	if err != nil {
		return fmt.Errorf("count identity-failed attempts: %w", err)
	}
	if failedAttempts < maxEpisodeAttempts {
		if err := episodeledger.RetainForRetry(ctx, tx, episodeID); err != nil {
			return fmt.Errorf("retain episode for retry: %w", err)
		}
		return nil
	}
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, episodeID, 0, r.runtimeNow()); err != nil {
			return fmt.Errorf("settle exhausted episode cost: %w", err)
		}
	}
	if err := episodeledger.Conclude(ctx, tx, episodeID, r.runtimeNow(), terminalJSON); err != nil {
		return fmt.Errorf("conclude identity-failed episode: %w", err)
	}
	return nil
}

func countFailedAttempts(ctx context.Context, tx *sql.Tx, episodeID string) (int, error) {
	var failedAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_attempts WHERE episode_id = ? AND status IN ('failed', 'timed_out', 'cancelled')`, episodeID).Scan(&failedAttempts); err != nil {
		return 0, fmt.Errorf("count failed attempts: %w", err)
	}
	return failedAttempts, nil
}
