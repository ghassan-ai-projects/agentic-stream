package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// Epochs is the configuration of the epoch drain/kill record. A nil Epochs is
// unconfigured and every operation refuses it.
type Epochs struct {
	Store store.Store
	Now   func() time.Time
}

func (e *Epochs) configured() bool { return e != nil && e.Store.Configured() }

// Kill marks the epoch killed: every later decision under it is refused, and
// in-flight episodes of that epoch are marked superseded so the runner cancels
// their provider calls.
func Kill(ctx context.Context, e *Epochs, epoch string) error {
	if !e.configured() || epoch == "" {
		return domain.ErrEpochControlNotConfigured
	}
	now := e.Now()
	if err := e.Store.WithTx(ctx, func(tx *store.Tx) error { return killTx(ctx, tx, epoch, now) }); err != nil {
		return fmt.Errorf("kill epoch %q: %w", epoch, err)
	}
	return nil
}

// killTx records the kill, cancels the epoch's in-flight episodes, and
// releases the cost reservations of admitted episodes that never started, in
// the transaction that writes the terminal kill record.
func killTx(ctx context.Context, tx *store.Tx, epoch string, now time.Time) error {
	if err := recordState(ctx, tx, epoch, domain.EpochKilled, now); err != nil {
		return err
	}
	// Read the unstarted admitted episodes before supersession rewrites their
	// lifecycle.
	unstarted, err := tx.UnstartedReservedEpisodes(ctx, epoch)
	if err != nil {
		return err
	}
	if err := tx.SupersedeEpoch(ctx, epoch, now); err != nil {
		return err
	}
	return releaseEpisodeCosts(ctx, tx, unstarted, now)
}

func releaseEpisodeCosts(ctx context.Context, tx *store.Tx, episodeIDs []string, now time.Time) error {
	for _, episodeID := range episodeIDs {
		if err := Settle(ctx, tx, episodeID, 0, kernel.FormatTime(now)); err != nil {
			return fmt.Errorf("release admitted episode cost %s: %w", episodeID, err)
		}
	}
	return nil
}

// Drain marks the epoch draining: new episodes are refused at admission.
func Drain(ctx context.Context, e *Epochs, epoch string) error {
	if !e.configured() || epoch == "" {
		return domain.ErrEpochControlNotConfigured
	}
	now := e.Now()
	if err := e.Store.WithTx(ctx, func(tx *store.Tx) error {
		return recordState(ctx, tx, epoch, domain.EpochDraining, now)
	}); err != nil {
		return fmt.Errorf("record epoch control: %w", err)
	}
	return nil
}

func recordState(ctx context.Context, tx *store.Tx, epoch, state string, now time.Time) error {
	if err := domain.CheckControllable(state); err != nil {
		return err
	}
	return tx.RecordEpochState(ctx, epoch, state, now)
}

// State returns "draining", "killed", or "" when the epoch is uncontrolled.
func State(ctx context.Context, e *Epochs, epoch string) (string, error) {
	if !e.configured() || epoch == "" {
		return "", domain.ErrEpochControlNotConfigured
	}
	state, _, err := e.Store.Autocommit().EpochState(ctx, epoch, "epoch control")
	return state, err
}
