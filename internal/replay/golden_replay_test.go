package replay_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
)

func TestGoldenTracesAreDeterministic(t *testing.T) {
	t.Parallel()
	traces := []string{
		"../../examples/predictive-maintenance/testdata/trace-opening.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-watch.jsonl",
		"../../examples/predictive-maintenance/testdata/trace-heartbeat.jsonl",
	}
	for _, tracePath := range traces {
		t.Run(filepath.Base(tracePath), func(t *testing.T) {
			t.Parallel()
			if _, err := os.Stat(tracePath); err != nil {
				t.Fatalf("trace file %s: %v", tracePath, err)
			}
			results, err := replay.RunNTimes(seededContext(t), replay.Request{SpecPath: predictiveSpec, TracePath: tracePath, TenantID: "default"}, 3)
			if err != nil {
				t.Fatalf("replay %s: %v", tracePath, err)
			}
			if len(results) != 3 || !replay.AllHashesEqual(results) {
				t.Fatalf("deterministic replay failed for %s: %d results, hashes differ across runs: %+v", tracePath, len(results), results)
			}
			if results[0].VersionsHash == "" || results[0].EventsProcessed == 0 {
				t.Fatalf("%s: empty result %+v", tracePath, results[0])
			}
		})
	}
}

func TestAllHashesEqualDetectsADifferingRun(t *testing.T) {
	t.Parallel()
	same := []replay.Result{{VersionsHash: "sha256:a"}, {VersionsHash: "sha256:a"}}
	if !replay.AllHashesEqual(same) || !replay.AllHashesEqual(nil) {
		t.Fatal("identical hashes, or no results, reported as different")
	}
	if replay.AllHashesEqual([]replay.Result{{VersionsHash: "sha256:a"}, {VersionsHash: "sha256:b"}}) {
		t.Fatal("different hashes reported as equal")
	}
}

func TestReplayRejectsExistingDatabaseAndSidecars(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing := replay.Request{DBPath: filepath.Join(dir, "replay.db"), SpecPath: predictiveSpec, TracePath: predictiveTrace, TenantID: "default"}
	if err := os.WriteFile(existing.DBPath, []byte("not a replay database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := replay.Run(t.Context(), existing); err == nil || !strings.Contains(err.Error(), "reserve fresh database path") {
		t.Fatalf("replay over an existing database = %v, want reserve fresh database path", err)
	}
	withSidecar := existing
	withSidecar.DBPath = filepath.Join(dir, "sidecar.db")
	if err := os.WriteFile(withSidecar.DBPath+"-wal", []byte("sidecar"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := replay.Run(t.Context(), withSidecar); err == nil || !strings.Contains(err.Error(), "fresh database sidecar already exists") {
		t.Fatalf("replay over an existing WAL sidecar = %v, want fresh database sidecar already exists", err)
	}
}

func TestRunMigratesAFreshIsolatedDatabaseItself(t *testing.T) {
	t.Parallel()
	request := newRequest(t)
	result, err := replay.Run(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != replay.ModeDeterministic || result.EffectsAllowed || result.WorkerInvoked || result.VersionCount == 0 || result.VersionsHash == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(request.DBPath); err != nil {
		t.Fatalf("the replay left no isolated database to inspect: %v", err)
	}
}
