package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// A debounced trigger queues its episode for later. On a quiet live source no
// new evidence arrives to advance the pipeline, so the scheduled advance must
// start it once it is due, and must not start it early.
func TestAdvanceEveryStartsDueCognitionWithoutNewEvidence(t *testing.T) {
	t.Parallel()
	db := openModeDB(t, "advance.db")
	compiled := modeCompiledSpec("native", "active")
	compiled.Cognition.Triggers[0].Debounce = "5s"
	clock := sources.NewVirtual(time.Date(2026, 8, 12, 0, 0, 2, 0, time.UTC))
	pipeline := newModePipeline(t, db, compiled, runtime.PipelineConfig{OwnerEpoch: "epoch-advance", Clock: clock})

	ingested, err := pipeline.RunJSONL(t.Context(), modeTraceFile(t, 15))
	if err != nil || ingested.EpisodesAdmitted != 0 {
		t.Fatalf("debounced trigger must wait: report=%+v err=%v", ingested, err)
	}
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- pipeline.AdvanceEvery(ctx, 5*time.Millisecond) }()
	time.Sleep(100 * time.Millisecond)
	if episodes := countRows(t, db, "episodes"); episodes != 0 {
		t.Fatalf("advance before the debounce admitted %d episodes", episodes)
	}
	clock.Advance(6 * time.Second)
	waitForRows(t, db, "commands")
	stop()
	if err := <-done; err != nil {
		t.Fatalf("advance loop: %v", err)
	}
}

func TestAdvanceEveryRejectsANonPositiveInterval(t *testing.T) {
	t.Parallel()
	db := openModeDB(t, "advance-interval.db")
	pipeline := newModePipeline(t, db, modeCompiledSpec("native", "active"), runtime.PipelineConfig{OwnerEpoch: "epoch-interval"})
	if err := pipeline.AdvanceEvery(t.Context(), 0); err == nil {
		t.Fatal("a zero interval must be refused")
	}
}

func countRows(t *testing.T, db *storage.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil { //nolint:gosec // fixed table names
		t.Fatal(err)
	}
	return count
}

// waitForRows waits until table has a row, as a scheduled advance creates it.
func waitForRows(t *testing.T, db *storage.DB, table string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if countRows(t, db, table) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %s row appeared", table)
}
