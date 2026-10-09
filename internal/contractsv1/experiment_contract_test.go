package contractsv1_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// Two contract files leave this repository by path or by copy, and the
// real-world-sensor experiment depends on both. Change them only together with
// the named consumer. See
// docs/unfinished-work-review-2026-10-08/EXPERIMENT_COMPATIBILITY.md (E7, E8).
const (
	// thermalCatalogPath is passed to `serve --device-catalog` by the
	// real-world-sensor RUNBOOK-G1; its digest is pinned by the device module.
	thermalCatalogPath = "internal/domain/conformance/v1/thermal-capability-catalog.json"
	// workerProtocolPath is vendored byte for byte by Tamoz
	// (gems/tamoz-stream/contracts/runtime-v1.proto).
	workerProtocolPath   = "../../docs/design/contracts/runtime-v1.proto"
	workerProtocolSHA256 = "621ae92f9ed3f059c5dee46ae168fd10bedc8490e1c6ceaad14a39fefe1a7688"
)

func TestExperimentThermalCatalogKeepsItsPath(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(thermalCatalogPath); err != nil {
		t.Fatalf("thermal catalog moved; the real-world-sensor RUNBOOK-G1 passes internal/contractsv1/%s to serve --device-catalog: %v", thermalCatalogPath, err)
	}
}

func readWorkerProtocol(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(workerProtocolPath)
	if err != nil {
		t.Fatalf("read runtime protocol: %v", err)
	}
	return string(data)
}

func TestExperimentWorkerProtocolIsUnchanged(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte(readWorkerProtocol(t)))
	if got := hex.EncodeToString(sum[:]); got != workerProtocolSHA256 {
		t.Errorf("runtime-v1.proto changed (sha256 %s); update Tamoz's vendored copy in the same change, then this pin", got)
	}
}

func TestWorkerProtocolKeepsTheFrozenBoundary(t *testing.T) {
	t.Parallel()
	proto := readWorkerProtocol(t)
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
		"TERMINAL_STATUS_CANCELLED = 3;", //nolint:misspell // Frozen protobuf enum.
		"TERMINAL_STATUS_FAILED = 4;",
		"TERMINAL_STATUS_TIMED_OUT = 5;",
		"TERMINAL_STATUS_BUDGET_EXHAUSTED = 6;",
		"string tracestate = 34;",
		"ArtifactManifest artifact_manifest = 5;",
	}
	for _, want := range required {
		if !strings.Contains(proto, want) {
			t.Errorf("protocol is missing frozen requirement %q", want)
		}
	}
	forbidden := []string{
		"TERMINAL_STATUS_DECIDED",
		"TERMINAL_STATUS_NO_ACTION",
		"TERMINAL_STATUS_NEEDS_HUMAN",
		"TERMINAL_STATUS_PENDING_VERIFICATION",
	}
	for _, name := range forbidden {
		if strings.Contains(proto, name) {
			t.Errorf("protocol still exposes stream-owned terminal state %q", name)
		}
	}
}
