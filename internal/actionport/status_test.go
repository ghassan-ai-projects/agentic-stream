package actionport_test

import (
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestTheFacadeReportsTheStatesAwaitingReconciliation(t *testing.T) {
	t.Parallel()
	want := []string{actionport.CommandOutcomeUnknown, actionport.CommandReconciling, actionport.CommandManualReview}
	got := actionport.UnresolvedCommandStatuses()
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("UnresolvedCommandStatuses = %v, want %v", got, want)
	}
	if !actionport.IsUnresolvedCommandStatus(actionport.CommandReconciling) || actionport.IsUnresolvedCommandStatus(actionport.CommandSucceeded) {
		t.Fatal("IsUnresolvedCommandStatus disagrees with the unresolved set")
	}
}
