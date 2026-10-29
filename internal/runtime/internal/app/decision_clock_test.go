package app_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var (
	worldNow = time.Date(2026, 8, 12, 0, 0, 2, 0, time.UTC)
	wallNow  = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
)

type splitClocks struct {
	pipeline       *runtime.Pipeline
	db             *storage.DB
	wall, decision *sources.Virtual
}

func openSplitClocks(t *testing.T, changes ...specChange) splitClocks {
	t.Helper()
	wall, decision := sources.NewVirtual(wallNow), sources.NewVirtual(worldNow)
	pipeline, db := openPipeline(t, thingSpec("native", "active", changes...), runtime.PipelineConfig{OwnerEpoch: "epoch-decision", Clock: wall, DecisionClock: decision})
	return splitClocks{pipeline: pipeline, db: db, wall: wall, decision: decision}
}

func withExpiresAfter(expiresAfter string) specChange {
	return func(s *spec.CompiledSpec) { s.Cognition.Triggers[0].ExpiresAfter = expiresAfter }
}

func (c splitClocks) quietBatch(t *testing.T) runtime.PipelineReport {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "quiet.jsonl")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := c.pipeline.RunJSONL(t.Context(), empty)
	if err != nil {
		t.Fatalf("a quiet batch: %v", err)
	}
	return report
}

func TestADebouncedItemBecomesDueWhenTheDecisionClockPassesItsWindowWhateverTheWallDoes(t *testing.T) {
	t.Parallel()
	c := openSplitClocks(t, withDebounce("1m"))
	if report, err := c.pipeline.RunJSONL(t.Context(), highLevelTrace(t)); err != nil || report.EpisodesAdmitted != 0 {
		t.Fatalf("a debounced trigger must wait while its window runs: report=%+v err=%v", report, err)
	}
	c.wall.Advance(24 * time.Hour)
	c.decision.Advance(59 * time.Second)
	if report := c.quietBatch(t); report.EpisodesAdmitted != 0 {
		t.Fatalf("admitted %d episodes before the decision clock passed the window", report.EpisodesAdmitted)
	}
	c.decision.Advance(time.Second)
	if report := c.quietBatch(t); report.EpisodesAdmitted != 1 {
		t.Fatalf("admitted %d episodes once the decision clock passed the window, want 1", report.EpisodesAdmitted)
	}
}

func TestAnItemExpiresOnTheDecisionClockAndNotOnTheWall(t *testing.T) {
	t.Parallel()
	c := openSplitClocks(t, withDebounce("5s"), withExpiresAfter("10s"))
	if _, err := c.pipeline.RunJSONL(t.Context(), highLevelTrace(t)); err != nil {
		t.Fatal(err)
	}
	c.wall.Advance(24 * time.Hour)
	c.decision.Advance(16 * time.Second)
	if report := c.quietBatch(t); report.EpisodesAdmitted != 0 {
		t.Fatalf("admitted %d episodes of an item whose expiry the decision clock passed", report.EpisodesAdmitted)
	}
	if status := scalar[string](t, c.db, "SELECT status FROM scheduler_items"); status == "admitted" {
		t.Fatalf("the item is %s, want it expired", status)
	}
}

func TestAnItemWithinItsExpiryIsAdmittedAfterTheWallPassedItsExpiry(t *testing.T) {
	t.Parallel()
	c := openSplitClocks(t, withDebounce("5s"), withExpiresAfter("10s"))
	if _, err := c.pipeline.RunJSONL(t.Context(), highLevelTrace(t)); err != nil {
		t.Fatal(err)
	}
	c.wall.Advance(24 * time.Hour)
	c.decision.Advance(6 * time.Second)
	if report := c.quietBatch(t); report.EpisodesAdmitted != 1 {
		t.Fatalf("admitted %d episodes inside the item's window on the decision clock, want 1", report.EpisodesAdmitted)
	}
}

func TestSchedulingRecordsCarryTheDecisionClockTimeAndTheEpisodeTheWallTime(t *testing.T) {
	t.Parallel()
	c := openSplitClocks(t, withDebounce("5s"))
	if _, err := c.pipeline.RunJSONL(t.Context(), highLevelTrace(t)); err != nil {
		t.Fatal(err)
	}
	c.decision.Advance(6 * time.Second)
	if report := c.quietBatch(t); report.EpisodesAdmitted != 1 {
		t.Fatalf("admitted %d episodes, want 1", report.EpisodesAdmitted)
	}
	for name, query := range map[string]string{
		"the item's creation":   "SELECT created_at FROM scheduler_items",
		"the item's window":     "SELECT not_before FROM scheduler_items",
		"the trigger's verdict": "SELECT evaluated_at FROM trigger_evaluations",
		"the episode admission": "SELECT accepted_at FROM episodes",
	} {
		if got := scalar[string](t, c.db, query)[:10]; got != "2026-08-12" {
			t.Errorf("%s is stamped %s, want a day of the decision clock", name, got)
		}
	}
	if got := scalar[string](t, c.db, "SELECT started_at FROM episode_attempts")[:10]; got != "2026-10-28" {
		t.Errorf("the episode attempt starts %s, want a day of the wall clock", got)
	}
}
