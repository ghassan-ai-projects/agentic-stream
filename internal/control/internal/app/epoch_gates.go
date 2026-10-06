package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// AssertDecision refuses a decision under a killed epoch. It is the
// decision-boundary gate: the episode's RECORDED policy_epoch is checked, so a
// kill refuses in-flight episodes even when the worker keeps producing.
func AssertDecision(ctx context.Context, e *Epochs, episodeEpoch string) error {
	if !e.configured() {
		return domain.ErrEpochControlNotConfigured
	}
	if episodeEpoch == "" {
		return domain.ErrEpochUnbound
	}
	state, err := State(ctx, e, episodeEpoch)
	if err != nil {
		return err
	}
	return domain.RefuseDecision(state)
}

// AssertDecisionTx performs the decision-boundary check on an existing
// transaction so the kill observation and the terminal writes share one SQLite
// snapshot.
func AssertDecisionTx(ctx context.Context, e *Epochs, tx *store.Tx, episodeEpoch string) error {
	if !e.configured() || !tx.Open() {
		return domain.ErrEpochControlNotConfigured
	}
	if episodeEpoch == "" {
		return domain.ErrEpochUnbound
	}
	state, _, err := tx.EpochState(ctx, episodeEpoch, "epoch control")
	if err != nil {
		return err
	}
	return domain.RefuseDecision(state)
}

// AssertOrdinaryTx refuses ordinary action work for an epoch that is draining
// or killed. It is transaction-scoped so target claims and the runtime owner
// assertion share one SQLite read boundary.
func AssertOrdinaryTx(ctx context.Context, e *Epochs, tx *store.Tx, epoch string) error {
	if !e.configured() || !tx.Open() || epoch == "" {
		return domain.ErrEpochControlNotConfigured
	}
	state, found, err := tx.EpochState(ctx, epoch, "ordinary epoch control")
	if err != nil || !found {
		return err
	}
	return domain.RefuseOrdinary(state)
}

// AssertAdmission refuses NEW episodes while draining or killed.
func AssertAdmission(ctx context.Context, e *Epochs, currentEpoch string) error {
	state, err := State(ctx, e, currentEpoch)
	if err != nil {
		return err
	}
	return domain.RefuseAdmission(state)
}
