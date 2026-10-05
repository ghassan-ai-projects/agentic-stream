package episodes

import (
	"context"
	"errors"
	"fmt"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
)

// epochRefusal projects the runtime control's decision-epoch assertion into
// the durable refusal reasons, keeping the control package out of the use
// cases: nil control never refuses, an unbound epoch refuses as epoch_unbound,
// a killed epoch as epoch_killed, and an unexpected control error fails the
// dispatch.
func epochRefusal(control *runtimecontrol.EpochControl) func(context.Context, *store.Tx, string) (string, error) {
	return func(ctx context.Context, tx *store.Tx, policyEpoch string) (string, error) {
		if control == nil {
			return "", nil
		}
		if policyEpoch == "" {
			return "epoch_unbound", nil
		}
		return refusalReason(tx.AssertDecisionEpoch(ctx, control.AssertDecisionTx, policyEpoch))
	}
}

func refusalReason(epochErr error) (string, error) {
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
}
