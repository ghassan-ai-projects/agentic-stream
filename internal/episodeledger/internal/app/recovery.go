package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// RecoverUnfinishedAttempts abandons active attempts owned by an older or
// missing runtime epoch. It preserves the episode and fence so the next
// StartAttempt allocates fence+1. Canceling attempts abandon their episode
// because cancellation is an explicit terminal decision, not a retry signal.
// When a settler is given, reservations of permanently abandoned episodes are
// released; requeued episodes keep theirs for the next fenced attempt.
func RecoverUnfinishedAttempts(ctx context.Context, tx *store.Tx, currentEpoch string, now time.Time, costs store.Settler) (domain.RecoveryReport, error) {
	if !tx.Open() || currentEpoch == "" {
		return domain.RecoveryReport{}, fmt.Errorf("recovery transaction and current epoch are required")
	}
	attempts, err := tx.UnfinishedAttempts(ctx, currentEpoch)
	if err != nil {
		return domain.RecoveryReport{}, err
	}
	recovery := &attemptRecovery{tx: tx, now: now, costs: costs}
	for _, item := range attempts {
		if err := recovery.recover(ctx, item); err != nil {
			return domain.RecoveryReport{}, err
		}
	}
	return recovery.report, nil
}

// attemptRecovery abandons prior-owner attempts inside one recovery
// transaction and tallies the result.
type attemptRecovery struct {
	tx     *store.Tx
	now    time.Time
	costs  store.Settler
	report domain.RecoveryReport
}

func (r *attemptRecovery) recover(ctx context.Context, item domain.UnfinishedAttempt) error {
	terminal, err := item.RecoveryTerminal()
	if err != nil {
		return err
	}
	rows, err := r.tx.AbandonUnfinishedAttempt(ctx, item.AttemptID, item.EpisodeID, r.now, terminal)
	if err != nil || rows != 1 {
		return err
	}
	r.report.AbandonedAttempts++
	if item.Canceling() {
		return r.abandonCancelingEpisode(ctx, item.EpisodeID, terminal)
	}
	return r.countRequeued(ctx, item.EpisodeID)
}

// abandonCancelingEpisode ends an episode whose attempt was being canceled and
// releases its cost reservation.
func (r *attemptRecovery) abandonCancelingEpisode(ctx context.Context, episodeID string, terminal []byte) error {
	rows, err := r.tx.AbandonOpenEpisode(ctx, episodeID, r.now, terminal)
	if err != nil {
		return err
	}
	if rows == 1 {
		r.report.AbandonedEpisodes++
	}
	if r.costs == nil {
		return nil
	}
	if err := r.tx.SettleEpisodeCost(ctx, r.costs, episodeID, r.now); err != nil {
		return fmt.Errorf("settle abandoned episode cost %s: %w", episodeID, err)
	}
	return nil
}

func (r *attemptRecovery) countRequeued(ctx context.Context, episodeID string) error {
	lifecycle, err := r.tx.EpisodeLifecycle(ctx, episodeID)
	if err != nil {
		return err
	}
	if domain.IsRequeued(lifecycle) {
		r.report.RequeuedEpisodes++
	}
	return nil
}
