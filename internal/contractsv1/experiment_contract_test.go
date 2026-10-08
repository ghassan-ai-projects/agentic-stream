package contractsv1_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
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

func TestExperimentWorkerProtocolIsUnchanged(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(workerProtocolPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != workerProtocolSHA256 {
		t.Errorf("runtime-v1.proto changed (sha256 %s); update Tamoz's vendored copy in the same change, then this pin", got)
	}
}
