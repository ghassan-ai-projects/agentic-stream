package replay_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/replay"
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
