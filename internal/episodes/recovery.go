package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
)

// RecoveryReport describes active attempt state abandoned during a runtime
// restart. Recovery is transaction-scoped so callers can commit it together
// with evidence-call recovery and ownership claim.
type RecoveryReport struct {
	AbandonedAttempts int
	RequeuedEpisodes  int
	AbandonedEpisodes int
}

// RecoverUnfinishedAttempts abandons active attempts owned by an older or
// missing runtime epoch. It preserves the episode and fence so the next
// StartAttempt allocates fence+1. Cancelling attempts abandon their episode
// because cancellation is an explicit terminal decision, not an automatic
// retry signal.
func RecoverUnfinishedAttempts(ctx context.Context, tx *sql.Tx, currentEpoch string, now time.Time) (RecoveryReport, error) {
	return RecoverUnfinishedAttemptsWithCost(ctx, tx, currentEpoch, now, nil)
}

// RecoverUnfinishedAttemptsWithCost also releases reservations for attempts
// that are permanently abandoned during restart. Requeued episodes retain
// their reservation for the next fenced attempt.
func RecoverUnfinishedAttemptsWithCost(ctx context.Context, tx *sql.Tx, currentEpoch string, now time.Time, costs *costcontrol.Controller) (RecoveryReport, error) {
	if tx == nil || currentEpoch == "" {
		return RecoveryReport{}, fmt.Errorf("recovery transaction and current epoch are required")
	}
	now = now.UTC()
	rows, err := tx.QueryContext(ctx, `
		SELECT attempt_id, episode_id, status, COALESCE(owner_epoch, '')
		FROM episode_attempts
		WHERE status IN ('dispatched', 'running', 'cancelling')
		  AND (owner_epoch IS NULL OR owner_epoch <> ?)`, currentEpoch)
	if err != nil {
		return RecoveryReport{}, fmt.Errorf("list unfinished attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type unfinished struct {
		attemptID, episodeID, status, ownerEpoch string
	}
	var attempts []unfinished
	for rows.Next() {
		var item unfinished
		if err := rows.Scan(&item.attemptID, &item.episodeID, &item.status, &item.ownerEpoch); err != nil {
			return RecoveryReport{}, fmt.Errorf("scan unfinished attempt: %w", err)
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		return RecoveryReport{}, fmt.Errorf("iterate unfinished attempts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return RecoveryReport{}, fmt.Errorf("close unfinished attempts: %w", err)
	}

	var report RecoveryReport
	for _, item := range attempts {
		terminal, err := json.Marshal(map[string]any{
			"status":               AttemptAbandoned,
			"reason":               "runtime_restart",
			"previous_owner_epoch": item.ownerEpoch,
		})
		if err != nil {
			return RecoveryReport{}, fmt.Errorf("marshal recovery terminal: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE episode_attempts
			SET status = 'abandoned', ended_at = ?, terminal_json = ?
			WHERE attempt_id = ? AND episode_id = ?
			  AND status IN ('dispatched', 'running', 'cancelling')`,
			formatTime(now), terminal, item.attemptID, item.episodeID)
		if err != nil {
			return RecoveryReport{}, fmt.Errorf("abandon attempt %s: %w", item.attemptID, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return RecoveryReport{}, fmt.Errorf("count abandoned attempt %s: %w", item.attemptID, err)
		}
		if count != 1 {
			continue
		}
		report.AbandonedAttempts++
		if item.status == string(AttemptCancelling) {
			result, err := tx.ExecContext(ctx, `
				UPDATE episodes
				SET lifecycle_status = 'abandoned', ended_at = ?,
				    terminal_json = ?
				WHERE episode_id = ? AND lifecycle_status NOT IN
				    ('concluded', 'closed', 'superseded', 'expired', 'abandoned')`,
				formatTime(now), terminal, item.episodeID)
			if err != nil {
				return RecoveryReport{}, fmt.Errorf("abandon cancelling episode %s: %w", item.episodeID, err)
			}
			if count, err := result.RowsAffected(); err == nil && count == 1 {
				report.AbandonedEpisodes++
			}
			if costs != nil {
				if err := costs.Settle(ctx, tx, item.episodeID, 0, formatTime(now)); err != nil {
					return RecoveryReport{}, fmt.Errorf("settle abandoned episode cost %s: %w", item.episodeID, err)
				}
			}
		} else {
			var lifecycle LifecycleStatus
			if err := tx.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", item.episodeID).Scan(&lifecycle); err != nil {
				return RecoveryReport{}, fmt.Errorf("load recovered episode %s: %w", item.episodeID, err)
			}
			if lifecycle == LifecycleAdmitted || lifecycle == LifecycleRunning {
				report.RequeuedEpisodes++
			}
		}
	}
	return report, nil
}
