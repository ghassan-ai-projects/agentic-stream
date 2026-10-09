package domain_test

import (
	"os"
	"strings"
	"testing"
)

func TestCompileRefusesIntentPoliciesThePolicyPlaneDoesNotEnforce(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../../examples/thermal-chamber/zone-thermal.situation.yaml")
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
			_, err := compileSource(t, edited)
			if err == nil || !strings.Contains(err.Error(), "schema validation") {
				t.Fatalf("an intent declared %q: err = %v, want a schema validation failure", unenforced, err)
			}
		})
	}
}
