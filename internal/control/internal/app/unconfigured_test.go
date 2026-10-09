package app

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

func TestUnconfiguredControlRefusesEveryOperation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	cases := map[string]struct {
		err  error
		want error
	}{
		"nil owner claim":           {Claim(ctx, nil, "e"), domain.ErrOwnerNotConfigured},
		"no-database owner renew":   {Renew(ctx, &Owner{}, "e"), domain.ErrOwnerNotConfigured},
		"no-database owner release": {Release(ctx, &Owner{}, "e"), domain.ErrOwnerNotConfigured},
		"closed-transaction assert": {Assert(ctx, &Owner{}, store.Join(nil), "e"), domain.ErrOwnerNotConfigured},
		"nil epochs kill":           {Kill(ctx, nil, "e"), domain.ErrEpochControlNotConfigured},
		"nil epochs drain":          {Drain(ctx, nil, "e"), domain.ErrEpochControlNotConfigured},
		"no-database admission":     {AssertAdmission(ctx, &Epochs{}, "e"), domain.ErrEpochControlNotConfigured},
		"no-database decision":      {AssertDecision(ctx, &Epochs{}, "e"), domain.ErrEpochControlNotConfigured},
		"unconfigured dispatch":     {AuthorizeDispatch(ctx, store.New(nil)), domain.ErrDispatchGateNotConfigured},
	}
	for name, tc := range cases {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s = %v, want %v", name, tc.err, tc.want)
		}
	}
}
