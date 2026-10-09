package app

import (
	"context"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
)

func TestOnlyARepeatableRefusalIsARuleFailure(t *testing.T) {
	t.Parallel()
	refused := errors.New("snapshot invalid")
	if failure, ruled := domain.AsRuleFailure(ruleFailure(t.Context(), "apply situation", refused)); !ruled || failure.Step != "apply situation" || !errors.Is(failure, refused) {
		t.Fatalf("a refusal = %v ruled %v, want a rule failure wrapping it", failure, ruled)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, ruled := domain.AsRuleFailure(ruleFailure(canceled, "apply situation", refused)); ruled {
		t.Fatal("a failure under a canceled context was set aside as repeatable")
	}
	if _, ruled := domain.AsRuleFailure(ruleFailure(t.Context(), "apply timers", context.DeadlineExceeded)); ruled {
		t.Fatal("a deadline was set aside as repeatable")
	}
}
