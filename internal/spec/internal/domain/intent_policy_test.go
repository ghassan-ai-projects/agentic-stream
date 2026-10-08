package domain_test

import (
	"context"
	"os"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

// Only `automatic` and `approval` are enforced by the policy plane. `deny` and
// `simulate` used to compile and then dispatch an R0/R1 intent automatically,
// failing open. An intent that must never run is simply not declared; nothing
// simulates an effect in governance. Both values are refused at compile time.
func TestCompileRefusesIntentPoliciesThePolicyPlaneDoesNotEnforce(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../../docs/design/examples/zone-thermal.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	const declared = "      policy: automatic\n      rateLimitPerHour: 30\n"
	if !strings.Contains(string(data), declared) {
		t.Fatal("zone-thermal set_indicator policy changed; update this test")
	}
	for _, unenforced := range []string{"deny", "simulate"} {
		t.Run(unenforced, func(t *testing.T) {
			t.Parallel()
			edited := strings.Replace(string(data), declared, "      policy: "+unenforced+"\n      rateLimitPerHour: 30\n", 1)
			if _, err := domain.NewCompiler().CompileBytes(context.Background(), []byte(edited), "zone-thermal.yaml"); err == nil {
				t.Fatalf("an intent declared %q compiled; the policy plane would dispatch it automatically", unenforced)
			}
		})
	}
}
