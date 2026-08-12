package contractsv1

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeProtocolFreezesWorkerBoundary(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not return the test path")
	}
	protoPath := filepath.Join(filepath.Dir(filename), "..", "..", "docs", "design", "contracts", "runtime-v1.proto")
	proto, err := os.ReadFile(protoPath)
	if err != nil {
		t.Fatalf("read runtime protocol: %v", err)
	}

	required := []string{
		`option go_package = "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1;runtimev1";`,
		"string contract_version = 2;",
		"string worker_id = 3;",
		"bool non_interactive = 4;",
		"bool emits_complete_replay_ledger = 7;",
		"EPISODE_KIND_DIAGNOSE = 1;",
		"EPISODE_KIND_RECONSIDER = 2;",
		"EPISODE_LANE_FAST = 1;",
		"EPISODE_LANE_DEEP = 2;",
		"EPISODE_LANE_BATCH = 3;",
		"RISK_CLASS_R4 = 5;",
		"string attempt_id = 4;",
		"uint64 fence = 5;",
		"TERMINAL_STATUS_PRODUCED = 1;",
		"TERMINAL_STATUS_DECLINED = 2;",
		"TERMINAL_STATUS_CANCELLED = 3;",
		"TERMINAL_STATUS_FAILED = 4;",
		"TERMINAL_STATUS_TIMED_OUT = 5;",
		"TERMINAL_STATUS_BUDGET_EXHAUSTED = 6;",
		"string tracestate = 34;",
		"ArtifactManifest artifact_manifest = 5;",
	}
	for _, want := range required {
		if !strings.Contains(string(proto), want) {
			t.Errorf("protocol is missing frozen requirement %q", want)
		}
	}

	for _, forbidden := range []string{
		"TERMINAL_STATUS_DECIDED",
		"TERMINAL_STATUS_NO_ACTION",
		"TERMINAL_STATUS_NEEDS_HUMAN",
		"TERMINAL_STATUS_PENDING_VERIFICATION",
	} {
		if strings.Contains(string(proto), forbidden) {
			t.Errorf("protocol still exposes stream-owned terminal state %q", forbidden)
		}
	}
}
