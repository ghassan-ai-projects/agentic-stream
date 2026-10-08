package domain

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// The thermal intent catalog is the vocabulary Tamoz must propose from
// (EXPERIMENT_DESIGN D2): select_thermal_mode {mode: hold | bounded_cooling},
// set_indicator and the watch fallback. Both experiment specs declare the same
// catalog, and Tamoz's thermal domain pins this digest on its side. A change
// here needs the matching Tamoz change.
const thermalIntentCatalogDigest = "sha256:729227ecb483d69d2060e89a170f0aa30105222760ddc018ea16f3f588a4df28"

func TestThermalIntentCatalogDigestParity(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"../../../../examples/real-world-sensor/zone-thermal-sim.situation.yaml",
		"../../../../examples/real-world-sensor/zone-thermal-bench.situation.yaml",
	} {
		compiled, err := spec.CompileFile(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		_, digest, err := CompileIntentCatalog(compiled.Actions.Intents)
		if err != nil {
			t.Fatal(err)
		}
		if digest != thermalIntentCatalogDigest {
			t.Errorf("%s intent catalog digest = %s; update Tamoz's thermal domain in the same change, then this pin", path, digest)
		}
	}
}
