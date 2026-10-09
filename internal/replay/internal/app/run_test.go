package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestRunReplaysTheTraceIntoAFreshIsolatedDatabase(t *testing.T) {
	t.Parallel()
	result, err := Run(seededContext(t), newRequest(t, fixtureSpec))
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != domain.ModeDeterministic || result.EffectsAllowed || result.WorkerInvoked {
		t.Fatalf("deterministic result crossed a boundary: %+v", result)
	}
	if result.EventsProcessed == 0 || result.VersionsHash == "" || result.VersionCount == 0 {
		t.Fatalf("result is empty: %+v", result)
	}
}

func TestRunRejectsExistingDatabase(t *testing.T) {
	t.Parallel()
	request := newRequest(t, fixtureSpec)
	if err := os.WriteFile(request.DBPath, []byte("not a replay database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), "open db") || !strings.Contains(err.Error(), "reserve fresh database path") {
		t.Fatalf("Run over an existing database = %v, want open db: reserve fresh database path", err)
	}
}

func TestRunWithoutAReadableSpecFailsBeforeAnyEvidenceIsIngested(t *testing.T) {
	t.Parallel()
	request := newRequest(t, filepath.Join(t.TempDir(), "missing.situation.yaml"))
	_, err := Run(seededContext(t), request)
	if err == nil || !strings.Contains(err.Error(), "compile spec") {
		t.Fatalf("Run with a missing spec = %v, want compile spec failure", err)
	}
}

func TestDeterministicModeEqualsRunAndNeverInvokesCognition(t *testing.T) {
	t.Parallel()
	ctx := seededContext(t)
	plain, err := Run(ctx, newRequest(t, fixtureSpec))
	if err != nil {
		t.Fatal(err)
	}
	request := newRequest(t, fixtureSpec)
	moded, err := RunMode(ctx, domain.ModeDeterministic, request)
	if err != nil {
		t.Fatal(err)
	}
	if moded.VersionsHash != plain.VersionsHash || moded.EventsProcessed != plain.EventsProcessed || moded.Mode != domain.ModeDeterministic {
		t.Fatalf("RunMode(deterministic) = %+v, Run = %+v", moded, plain)
	}
	if moded.WorkerInvoked || moded.EffectsAllowed || moded.CapabilityCalls != 0 {
		t.Fatalf("deterministic mode crossed a boundary: %+v", moded)
	}
	db := openReplayDatabase(t, request.DBPath)
	for _, table := range []string{"trigger_evaluations", "episodes", "intents", "commands", "outbox"} {
		if rows := countRows(t, db, table); rows != 0 {
			t.Fatalf("deterministic replay wrote %d rows into %s", rows, table)
		}
	}
}

func TestRunModeFailsClosedBeforeAnyWorkWithoutCapabilities(t *testing.T) {
	t.Parallel()
	for _, mode := range []domain.Mode{domain.ModeRecorded, domain.ModeShadow} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			request := newRequest(t, fixtureSpec)
			result, err := RunMode(t.Context(), mode, request)
			if !errors.Is(err, domain.ErrModeCapabilityRequired) {
				t.Fatalf("RunMode(%s) = %v, want ErrModeCapabilityRequired", mode, err)
			}
			if result.Mode != mode || result.EffectsAllowed || result.WorkerInvoked {
				t.Fatalf("failed-closed result = %+v", result)
			}
			if _, statErr := os.Stat(request.DBPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("a refused mode created the replay database: %v", statErr)
			}
		})
	}
}

func TestRunModeRefusesRemovedAndUnknownModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []domain.Mode{"counterfactual", "future"} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			_, err := RunMode(t.Context(), mode, newRequest(t, fixtureSpec))
			if !errors.Is(err, domain.ErrUnsupportedMode) {
				t.Fatalf("RunMode(%s) = %v, want ErrUnsupportedMode", mode, err)
			}
		})
	}
}

func TestRunModeRefusesMultipleCapabilitySets(t *testing.T) {
	t.Parallel()
	_, err := RunMode(t.Context(), domain.ModeRecorded, newRequest(t, fixtureSpec), domain.Capabilities{}, domain.Capabilities{})
	if err == nil || !strings.Contains(err.Error(), "at most one replay capability set") {
		t.Fatalf("two capability sets = %v, want at most one replay capability set", err)
	}
}

func TestRunNTimesRepeatsDeterministicallyInSeparateDatabases(t *testing.T) {
	t.Parallel()
	results, err := RunNTimes(seededContext(t), domain.Request{SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || !domain.AllHashesEqual(results) || results[0].VersionsHash == "" {
		t.Fatalf("repeated runs = %+v, want three identical non-empty hashes", results)
	}
}

func TestRunNTimesRejectsANonPositiveCountAndNamesTheFailedRun(t *testing.T) {
	t.Parallel()
	request := domain.Request{SpecPath: fixtureSpec, TracePath: fixtureTrace, TenantID: "default"}
	if _, err := RunNTimes(t.Context(), request, 0); err == nil || !strings.Contains(err.Error(), "n must be > 0") {
		t.Fatalf("n = 0 = %v, want n must be > 0", err)
	}
	request.SpecPath = filepath.Join(t.TempDir(), "missing.situation.yaml")
	if _, err := RunNTimes(seededContext(t), request, 2); err == nil || !strings.Contains(err.Error(), "run 0:") {
		t.Fatalf("a failing repeat = %v, want it attributed to run 0", err)
	}
}

func TestDeterministicBaselineProducesAValidatedRecommendation(t *testing.T) {
	t.Parallel()
	if _, err := NewDeterministicBaseline(nil); err == nil || !strings.Contains(err.Error(), "non-empty intent catalog") {
		t.Fatalf("a nil spec = %v, want a non-empty intent catalog requirement", err)
	}
	compiled, err := spec.CompileFile(t.Context(), fixtureSpec)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := NewDeterministicBaseline(compiled)
	if err != nil {
		t.Fatalf("create baseline: %v", err)
	}
	input := domain.ShadowInput{
		TenantID: "default", EpisodeKey: "episode-1", EpisodeID: "episode-1", SituationID: "situation-1",
		SituationVersion: 1, AttemptID: "attempt-1", Fence: 1,
		SnapshotDigest: "sha256:" + strings.Repeat("1", 64),
		SnapshotJSON:   []byte(`{"entity":{"id":"motor-1"},"phase":"warning"}`),
	}
	output, err := baseline.ExecuteBaseline(t.Context(), input)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if output.ExecutorVersion == "" || output.DecisionSHA256 == "" || !strings.Contains(string(output.DecisionJSON), "create_maintenance_ticket") {
		t.Fatalf("baseline did not produce a create_maintenance_ticket recommendation: %+v", output)
	}
}
