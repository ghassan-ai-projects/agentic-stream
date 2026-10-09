package actionport_test

import (
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestUnresolvedCommandStatusesAreExactlyTheStatesAwaitingReconciliation(t *testing.T) {
	t.Parallel()
	for status, unresolved := range map[string]bool{
		actionport.CommandPending: false, actionport.CommandDispatching: false,
		actionport.CommandSucceeded: false, actionport.CommandFailed: false,
		actionport.CommandReconciling: true, actionport.CommandOutcomeUnknown: true, actionport.CommandManualReview: true,
	} {
		if got := actionport.IsUnresolvedCommandStatus(status); got != unresolved {
			t.Fatalf("status %q: unresolved = %v, want %v", status, got, unresolved)
		}
		if got := slices.Contains(actionport.UnresolvedCommandStatuses(), status); got != unresolved {
			t.Fatalf("status %q: listed = %v, want %v", status, got, unresolved)
		}
	}
}

func TestUnresolvedCommandStatusesCannotBeMutatedByCallers(t *testing.T) {
	t.Parallel()
	actionport.UnresolvedCommandStatuses()[0] = "tampered"
	if slices.Contains(actionport.UnresolvedCommandStatuses(), "tampered") {
		t.Fatal("the unresolved set must be returned as a fresh slice")
	}
}
