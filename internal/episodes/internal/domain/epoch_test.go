package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestDecisionEpochRefusalNamesTheSentinelAndKeepsUnexpectedErrors(t *testing.T) {
	t.Parallel()
	unbound, killed, unexpected := errors.New("unbound"), errors.New("killed"), errors.New("store failed")
	for _, test := range []struct {
		err    error
		reason string
	}{{nil, ""}, {fmt.Errorf("wrapped: %w", unbound), "epoch_unbound"}, {fmt.Errorf("wrapped: %w", killed), "epoch_killed"}} {
		reason, err := DecisionEpochRefusal(test.err, unbound, killed)
		if err != nil || reason != test.reason {
			t.Fatalf("refusal=%s error=%v", reason, err)
		}
	}
	if reason, err := DecisionEpochRefusal(unexpected, unbound, killed); reason != "" || !errors.Is(err, unexpected) {
		t.Fatalf("unexpected refusal=%s error=%v", reason, err)
	}
}
