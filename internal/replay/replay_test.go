package replay_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestGoldenTracesAreDeterministic(t *testing.T) {
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	traces := []string{
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-watch.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl",
	}

	for _, tracePath := range traces {
		t.Run(filepath.Base(tracePath), func(t *testing.T) {
			if _, err := os.Stat(tracePath); err != nil {
				t.Fatalf("trace file %s: %v", tracePath, err)
			}

			results, err := replay.RunNTimes(ctx, specPath, tracePath, "default", 3)
			if err != nil {
				t.Fatalf("replay %s: %v", tracePath, err)
			}

			if !replay.AllHashesEqual(results) {
				t.Fatalf("deterministic replay failed for %s: hashes differ across runs", tracePath)
			}

			t.Logf("%s: processed=%d versions=%d hash=%s",
				filepath.Base(tracePath), results[0].EventsProcessed,
				results[0].VersionCount, results[0].VersionsHash)
		})
	}
}

func TestReplayModesHaveNoCredentialOrEffectorBoundary(t *testing.T) {
	ctx := context.Background()
	specPath := "../../docs/design/examples/predictive-maintenance.situation.yaml"
	tracePath := "../../examples/predictive-maintenance/testdata/trace-opening.jsonl"
	for _, mode := range []replay.Mode{replay.ModeDeterministic, replay.ModeRecorded, replay.ModeShadow, replay.ModeCounterfactual} {
		t.Run(string(mode), func(t *testing.T) {
			result, err := replay.RunMode(ctx, mode, filepath.Join(t.TempDir(), "replay.db"), specPath, tracePath, "default")
			if mode != replay.ModeDeterministic {
				if !errors.Is(err, replay.ErrModeCapabilityRequired) {
					t.Fatalf("expected explicit capability error, got %v", err)
				}
				if result.WorkerInvoked || result.EffectsAllowed {
					t.Fatalf("unsupported mode crossed an unsafe boundary: worker=%v effects=%v", result.WorkerInvoked, result.EffectsAllowed)
				}
				return
			}
			if err != nil {
				t.Fatalf("run mode: %v", err)
			}
			if result.Mode != mode {
				t.Fatalf("mode = %q, want %q", result.Mode, mode)
			}
			if result.WorkerInvoked || result.EffectsAllowed {
				t.Fatalf("mode crossed an unsafe boundary: worker=%v effects=%v", result.WorkerInvoked, result.EffectsAllowed)
			}
			if result.EventsProcessed == 0 {
				t.Fatal("mode processed no events")
			}
		})
	}
}

func TestReplayRejectsExistingDatabaseAndSidecars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.db")
	if err := os.WriteFile(path, []byte("not a replay database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := replay.Run(context.Background(), path,
		"../../docs/design/examples/predictive-maintenance.situation.yaml",
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default")
	if err == nil {
		t.Fatal("expected existing database to be rejected")
	}

	sidecar := filepath.Join(dir, "sidecar.db-wal")
	if err := os.WriteFile(sidecar, []byte("sidecar"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = replay.Run(context.Background(), filepath.Join(dir, "sidecar.db"),
		"../../docs/design/examples/predictive-maintenance.situation.yaml",
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default")
	if err == nil {
		t.Fatal("expected existing WAL sidecar to be rejected")
	}
}

func TestDeterministicReplayDoesNotInvokeCognition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stream-only.db")
	_, err := replay.Run(context.Background(), path,
		"../../docs/design/examples/predictive-maintenance.situation.yaml",
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl", "default")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen replay database: %v", err)
	}
	defer func() { _ = db.Close() }()
	var evaluations int
	if err := db.QueryRow("SELECT COUNT(*) FROM trigger_evaluations").Scan(&evaluations); err != nil {
		t.Fatalf("count trigger evaluations: %v", err)
	}
	if evaluations != 0 {
		t.Fatalf("deterministic replay created %d trigger evaluations", evaluations)
	}
}
