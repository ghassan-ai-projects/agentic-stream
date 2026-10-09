package app_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestDispatchQuarantinesAnEpisodeBoundToAStaleSituationVersion(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seed := newEpisodeSeed("epi-stale")
	seed.LiveVersion = 2
	seed.insert(t, db)
	executor := newRecordingExecutor()

	mustRunOnce(t, runnerWithEpochGate(db, executor))

	if got := lifecycleOf(t, db, "epi-stale"); got != "abandoned" {
		t.Fatalf("stale episode lifecycle = %q, want abandoned", got)
	}
	if got := terminalReasonOf(t, db, "epi-stale"); got != "stale_situation" {
		t.Fatalf("terminal reason = %q, want stale_situation", got)
	}
	if len(executor.requests) != 0 {
		t.Fatalf("the executor saw %d requests for a stale snapshot, want none", len(executor.requests))
	}
}

func TestDispatchRunsAnEpisodeBoundToTheLiveSituationVersion(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-fresh")
	executor := newRecordingExecutor()

	mustRunOnce(t, runnerWithEpochGate(db, executor))

	if len(executor.requests) != 1 || executor.requests[0].SituationVersion != 1 {
		t.Fatalf("executor requests = %+v, want exactly one at version 1", executor.requests)
	}
	if got := lifecycleOf(t, db, "epi-fresh"); got != "concluded" {
		t.Fatalf("lifecycle = %q, want concluded", got)
	}
}

func budgetOf(t *testing.T, db *storage.DB, episodeID string) any {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal(scalar[[]byte](t, db, "SELECT request_json FROM episodes WHERE episode_id = ?", episodeID), &request); err != nil {
		t.Fatal(err)
	}
	return request["budget"]
}

func TestDispatchNeverRewritesTheAdmittedBudget(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-budget")
	before := budgetOf(t, db, "epi-budget")

	mustRunOnce(t, runnerWithEpochGate(db, fixture.New()))

	if after := budgetOf(t, db, "epi-budget"); !reflect.DeepEqual(before, after) {
		t.Fatalf("budget changed across dispatch: before=%v after=%v", before, after)
	}
}

func TestDispatchRefusesADecisionThatArrivesAfterTheWallTimeBudget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		elapsed    time.Duration
		wantStatus string
		wantReason string
	}{
		{"well within the budget", 500 * time.Millisecond, "produced", ""},
		{"exactly at the budget", time.Second, "produced", ""},
		{"a moment past the budget", time.Second + time.Millisecond, "timed_out", "decision_after_deadline"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			seed := newEpisodeSeed("epi-slow")
			seed.WallTime = "1s"
			seed.insert(t, db)
			clock := sources.NewVirtual(time.Date(2026, 8, 12, 10, 0, 1, 0, time.UTC))
			executor := executorFunc(func(ctx context.Context, req *app.Request) (*app.Outcome, error) {
				clock.Advance(tc.elapsed)
				return fixture.New().Execute(ctx, req)
			})

			mustRunOnce(t, app.NewRunner(store.New(db), executor, clock, sources.Deterministic()))

			if got := attemptStatusOf(t, db, "epi-slow"); got != tc.wantStatus {
				t.Fatalf("attempt status = %q, want %q", got, tc.wantStatus)
			}
			if got := scalar[string](t, db, "SELECT COALESCE(json_extract(terminal_json, '$.reason'), '') FROM episode_attempts WHERE episode_id = 'epi-slow'"); got != tc.wantReason {
				t.Fatalf("attempt reason = %q, want %q", got, tc.wantReason)
			}
			if wantDecisions := map[string]int{"produced": 1, "timed_out": 0}[tc.wantStatus]; decisionCount(t, db, "epi-slow") != wantDecisions {
				t.Fatalf("decisions = %d, want %d", decisionCount(t, db, "epi-slow"), wantDecisions)
			}
		})
	}
}
