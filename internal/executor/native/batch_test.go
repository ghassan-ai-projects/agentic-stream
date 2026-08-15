package native_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/conformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/native"
)

func TestRunBatchReportsEveryCellIntentionToTreat(t *testing.T) {
	executor, err := native.New(native.Config{Provider: &native.DeterministicProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	requests := make([]*episodes.Request, 3)
	cellIDs := []string{"pilot.do-crash.001", "pilot.do-crash.002", "pilot.do-crash.003"}
	for i := range requests {
		requests[i] = conformance.FixtureRequest()
		requests[i].EpisodeID = cellIDs[i]
	}
	results, err := native.RunBatch(context.Background(), executor, requests, cellIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, result := range results {
		if result.Status != string(episodes.AttemptProduced) {
			t.Fatalf("expected produced outcome for %s, got %#v", result.CellID, result)
		}
		if len(result.Decision) == 0 {
			t.Fatalf("expected a decision for %s", result.CellID)
		}
	}
}

func TestRunBatchJSONRoundTrips(t *testing.T) {
	executor, err := native.New(native.Config{Provider: &native.DeterministicProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	raw, err := native.RunBatchJSON(context.Background(), executor, []*episodes.Request{request}, []string{"cell-1"})
	if err != nil {
		t.Fatal(err)
	}
	var results []native.BatchResult
	if err := json.Unmarshal(raw, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].CellID != "cell-1" {
		t.Fatalf("unexpected round-trip: %#v", results)
	}
}

func TestRunBatchRejectsMismatchedLengths(t *testing.T) {
	executor, err := native.New(native.Config{Provider: &native.DeterministicProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.RunBatch(context.Background(), executor, []*episodes.Request{conformance.FixtureRequest()}, nil)
	if err == nil {
		t.Fatal("expected a length mismatch error")
	}
}

type failingProvider struct{}

func (failingProvider) Name() string { return "failing" }
func (failingProvider) Stream(context.Context, native.ModelRequest) (native.ModelResponse, error) {
	return native.ModelResponse{}, errors.New("provider exploded")
}

// Intention-to-treat: a failed cell is REPORTED with its failure, never
// replaced or dropped — the batch has an entry for every cell.
func TestRunBatchReportsFailedCellsIntentionToTreat(t *testing.T) {
	executor, err := native.New(native.Config{Provider: failingProvider{}})
	if err != nil {
		t.Fatal(err)
	}
	request := conformance.FixtureRequest()
	results, err := native.RunBatch(context.Background(), executor, []*episodes.Request{request}, []string{"cell-fail-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly one result for one cell, got %d", len(results))
	}
	result := results[0]
	if result.Status != string(episodes.AttemptFailed) {
		t.Fatalf("expected AttemptFailed, got %q", result.Status)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != "provider_failed" {
		t.Fatalf("expected the executor's typed failure reason, got %#v", result.Reasons)
	}
	if len(result.Decision) != 0 {
		t.Fatal("a failed cell must not carry a decision")
	}
}
