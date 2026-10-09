package domain_test

import (
	"slices"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/actionport/internal/domain"
)

func TestUnresolvedCommandStatusesAreExactlyTheStatesAwaitingReconciliation(t *testing.T) {
	t.Parallel()
	for status, unresolved := range map[string]bool{
		domain.CommandPending: false, domain.CommandDispatching: false,
		domain.CommandSucceeded: false, domain.CommandFailed: false,
		domain.CommandReconciling: true, domain.CommandOutcomeUnknown: true, domain.CommandManualReview: true,
	} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			if got := domain.IsUnresolvedCommandStatus(status); got != unresolved {
				t.Errorf("unresolved = %v, want %v", got, unresolved)
			}
			if got := slices.Contains(domain.UnresolvedCommandStatuses(), status); got != unresolved {
				t.Errorf("listed = %v, want %v", got, unresolved)
			}
		})
	}
}

func TestUnresolvedCommandStatusesCannotBeMutatedByCallers(t *testing.T) {
	t.Parallel()
	domain.UnresolvedCommandStatuses()[0] = "tampered"
	if slices.Contains(domain.UnresolvedCommandStatuses(), "tampered") {
		t.Fatal("the unresolved set must be returned as a fresh slice")
	}
}
