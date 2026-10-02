package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
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
			if err := r.db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err == nil && lifecycle == string(LifecycleSuperseded) {
				cancel()
				return
			}
		}
	}
}

func executionFailureStatus(err error) AttemptStatus {
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return AttemptCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
		return AttemptTimedOut
	}
	return AttemptFailed
}

func executionFailureReason(err error) string {
	var budgetErr *budgetExceededError
	if errors.As(err, &budgetErr) {
		return "budget_exhausted"
	}
	var telemetryErr budgetTelemetryMissingError
	if errors.As(err, &telemetryErr) {
		return "budget_telemetry_missing"
	}
	switch executionFailureStatus(err) {
	case AttemptCancelled:
		return "worker_cancelled" //nolint:misspell // Durable protocol reason is frozen as cancelled.
	case AttemptTimedOut:
		return "worker_deadline_exceeded"
	default:
		return "worker_execution_failed"
	}
}

// failAttemptStatus fails the current attempt, then retries the episode or
// concludes it once the retry budget is spent.
func (r *Runner) failAttemptStatus(ctx context.Context, identity Identity, attemptStatus AttemptStatus, reason string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		terminalJSON, err := json.Marshal(map[string]any{"status": attemptStatus, "reason": reason})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if attemptStatus == AttemptCancelled {
			if err := r.markCancelling(ctx, tx, identity); err != nil {
				return err
			}
		}
		if err := TransitionAttempt(ctx, tx, identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish failed episode attempt: %w", err)
		}
		return r.retryOrConcludeFailedEpisode(ctx, tx, identity.EpisodeID)
	})
}

// markCancelling moves the attempt through AttemptCancelling, which the
// attempt lifecycle requires before AttemptCancelled.
func (r *Runner) markCancelling(ctx context.Context, tx *sql.Tx, identity Identity) error {
	var current AttemptStatus
	if err := tx.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&current); err != nil {
		return fmt.Errorf("read episode attempt status: %w", err)
	}
	if current == AttemptCancelling {
		return nil
	}
	if err := TransitionAttempt(ctx, tx, identity, AttemptCancelling, r.clk.Now(), nil); err != nil {
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
	superseded := lifecycle == string(LifecycleSuperseded)
	if !superseded && failedAttempts < maxEpisodeAttempts {
		if _, err := tx.ExecContext(ctx, `
			UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL
			WHERE episode_id = ?`,
			episodeID,
		); err != nil {
			return fmt.Errorf("update episode failed: %w", err)
		}
		return nil
	}
	if !superseded {
		terminalJSON, err := json.Marshal(map[string]any{"status": AttemptFailed, "reason": "attempt_retry_limit"})
		if err != nil {
			return fmt.Errorf("marshal retry limit terminal: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?", r.runtimeNow(), terminalJSON, episodeID); err != nil {
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

func (r *Runner) failAttemptWithRejection(ctx context.Context, current, incoming Identity, reason RejectionReason, detail string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		details, err := json.Marshal(map[string]any{"message": detail, "incoming_attempt_id": incoming.AttemptID, "incoming_fence": incoming.Fence})
		if err != nil {
			return fmt.Errorf("marshal identity rejection: %w", err)
		}
		if err := RecordRejection(ctx, tx, incoming, reason, details, r.clk.Now()); err != nil {
			return fmt.Errorf("record worker identity rejection: %w", err)
		}
		terminalJSON, err := json.Marshal(map[string]any{"status": AttemptFailed, "reason": detail})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if err := TransitionAttempt(ctx, tx, current, AttemptFailed, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish identity-failed attempt: %w", err)
		}
		failedAttempts, err := countFailedAttempts(ctx, tx, current.EpisodeID)
		if err != nil {
			return fmt.Errorf("count identity-failed attempts: %w", err)
		}
		if failedAttempts < maxEpisodeAttempts {
			if _, err := tx.ExecContext(ctx, `
				UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL
				WHERE episode_id = ?`, current.EpisodeID); err != nil {
				return fmt.Errorf("retain episode for retry: %w", err)
			}
			return nil
		}
		if r.cost != nil {
			if err := r.cost.Settle(ctx, tx, current.EpisodeID, 0, r.runtimeNow()); err != nil {
				return fmt.Errorf("settle exhausted episode cost: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?", r.runtimeNow(), terminalJSON, current.EpisodeID); err != nil {
			return fmt.Errorf("conclude identity-failed episode: %w", err)
		}
		return nil
	})
}

func countFailedAttempts(ctx context.Context, tx *sql.Tx, episodeID string) (int, error) {
	var failedAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_attempts WHERE episode_id = ? AND status IN ('failed', 'timed_out', 'cancelled')`, episodeID).Scan(&failedAttempts); err != nil {
		return 0, fmt.Errorf("count failed attempts: %w", err)
	}
	return failedAttempts, nil
}
