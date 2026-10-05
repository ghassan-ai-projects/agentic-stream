package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// withEpochRunner builds a runner with the kill gate wired over db.
func withEpochRunner(db *storage.DB, executor Executor, clk clock.Clock, idGen ids.Generator) *Runner {
	return withEpochControl(NewRunner(store.New(db), executor, clk, idGen), &runtimecontrol.EpochControl{DB: db})
}

// withEpochControl wires the production refusal mapping for app tests; the
// public facade injects the same projection at composition time.
func withEpochControl(r *Runner, control *runtimecontrol.EpochControl) *Runner {
	return r.WithEpochRefusal(func(ctx context.Context, tx *store.Tx, policyEpoch string) (string, error) {
		epochErr := runtimecontrol.ErrEpochUnbound
		if policyEpoch != "" {
			epochErr = tx.AssertDecisionEpoch(ctx, control.AssertDecisionTx, policyEpoch)
		}
		switch {
		case epochErr == nil:
			return "", nil
		case errors.Is(epochErr, runtimecontrol.ErrEpochUnbound):
			return "epoch_unbound", nil
		case errors.Is(epochErr, runtimecontrol.ErrEpochKilled):
			return "epoch_killed", nil
		default:
			return "", fmt.Errorf("assert decision epoch: %w", epochErr)
		}
	})
}
