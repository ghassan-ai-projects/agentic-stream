package actionport_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestUnknownOutcomePreservesCauseAndRequiresReconciliation(t *testing.T) {
	t.Parallel()
	ambiguous := &actionport.UnknownOutcomeError{Err: context.DeadlineExceeded}
	wrapped := fmt.Errorf("gateway failed: %w", ambiguous)
	if !actionport.IsUnknownOutcome(wrapped) || !errors.Is(wrapped, context.DeadlineExceeded) {
		t.Fatalf("ambiguous cause lost: %v", wrapped)
	}
	if ambiguous.Error() != "action outcome is unknown: context deadline exceeded" {
		t.Fatal(ambiguous.Error())
	}
	for _, err := range []error{nil, context.Canceled, errors.New("not sent")} {
		if actionport.IsUnknownOutcome(err) {
			t.Fatalf("pre-effect failure requires reconciliation: %v", err)
		}
	}
}

func TestUnknownOutcomeWithoutCauseStillHasExplicitClassification(t *testing.T) {
	t.Parallel()
	for _, err := range []*actionport.UnknownOutcomeError{nil, {}} {
		if err.Error() != "action outcome is unknown" || err.Unwrap() != nil {
			t.Fatalf("empty unknown outcome: %v", err)
		}
	}
}
