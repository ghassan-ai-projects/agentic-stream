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
// StartAttempt allocates fence+1. Canceling attempts abandon their episode
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
	attempts, err := listUnfinishedAttempts(ctx, tx, currentEpoch)
	if err != nil {
		return RecoveryReport{}, err
	}
	recovery := attemptRecovery{tx: tx, now: formatTime(now.UTC()), costs: costs}
	for _, item := range attempts {
		if err := recovery.recover(ctx, item); err != nil {
			return RecoveryReport{}, err
		}
	}
	return recovery.report, nil
}

type unfinishedAttempt struct {
	attemptID, episodeID, status, ownerEpoch string
}

func listUnfinishedAttempts(ctx context.Context, tx *sql.Tx, currentEpoch string) ([]unfinishedAttempt, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT attempt_id, episode_id, status, COALESCE(owner_epoch, '')
		FROM episode_attempts
		WHERE status IN ('dispatched', 'running', 'cancelling')
		  AND (owner_epoch IS NULL OR owner_epoch <> ?)`, currentEpoch)
	if err != nil {
		return nil, fmt.Errorf("list unfinished attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var attempts []unfinishedAttempt
	for rows.Next() {
		var item unfinishedAttempt
		if err := rows.Scan(&item.attemptID, &item.episodeID, &item.status, &item.ownerEpoch); err != nil {
			return nil, fmt.Errorf("scan unfinished attempt: %w", err)
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unfinished attempts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close unfinished attempts: %w", err)
	}
	return attempts, nil
}

// attemptRecovery abandons prior-owner attempts inside one recovery
// transaction and tallies the result.
type attemptRecovery struct {
	tx     *sql.Tx
	now    string
	costs  *costcontrol.Controller
	report RecoveryReport
}

func (r *attemptRecovery) recover(ctx context.Context, item unfinishedAttempt) error {
	terminal, err := json.Marshal(map[string]any{
		"status":               AttemptAbandoned,
		"reason":               "runtime_restart",
		"previous_owner_epoch": item.ownerEpoch,
	})
	if err != nil {
		return fmt.Errorf("marshal recovery terminal: %w", err)
	}
	result, err := r.tx.ExecContext(ctx, `
		UPDATE episode_attempts
		SET status = 'abandoned', ended_at = ?, terminal_json = ?
		WHERE attempt_id = ? AND episode_id = ?
		  AND status IN ('dispatched', 'running', 'cancelling')`,
		r.now, terminal, item.attemptID, item.episodeID)
	if err != nil {
		return fmt.Errorf("abandon attempt %s: %w", item.attemptID, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count abandoned attempt %s: %w", item.attemptID, err)
	}
	if count != 1 {
		return nil
	}
	r.report.AbandonedAttempts++
	if item.status == string(AttemptCancelling) {
		return r.abandonCancelingEpisode(ctx, item.episodeID, terminal)
	}
	return r.countRequeued(ctx, item.episodeID)
}

// abandonCancelingEpisode ends an episode whose attempt was being canceled:
// cancellation is a terminal decision, not a retry signal.
func (r *attemptRecovery) abandonCancelingEpisode(ctx context.Context, episodeID string, terminal []byte) error {
	result, err := r.tx.ExecContext(ctx, `
		UPDATE episodes
		SET lifecycle_status = 'abandoned', ended_at = ?,
		    terminal_json = ?
		WHERE episode_id = ? AND lifecycle_status NOT IN
		    ('concluded', 'closed', 'superseded', 'expired', 'abandoned')`,
		r.now, terminal, episodeID)
	if err != nil {
		return fmt.Errorf("abandon canceling episode %s: %w", episodeID, err)
	}
	if count, err := result.RowsAffected(); err == nil && count == 1 {
		r.report.AbandonedEpisodes++
	}
	if r.costs != nil {
		if err := r.costs.Settle(ctx, r.tx, episodeID, 0, r.now); err != nil {
			return fmt.Errorf("settle abandoned episode cost %s: %w", episodeID, err)
		}
	}
	return nil
}

func (r *attemptRecovery) countRequeued(ctx context.Context, episodeID string) error {
	var lifecycle LifecycleStatus
	if err := r.tx.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err != nil {
		return fmt.Errorf("load recovered episode %s: %w", episodeID, err)
	}
	if lifecycle == LifecycleAdmitted || lifecycle == LifecycleRunning {
		r.report.RequeuedEpisodes++
	}
	return nil
}
