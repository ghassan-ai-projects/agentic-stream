package policy_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// Every device command carries the policy digest, and the real-world-sensor
// bench gateway allow-lists it (arduino_gateway.py --device-policy-digest).
// The digest covers the policy document bound to a spec digest, so changing
// the document's rules changes the value every deployed gateway accepts.
// Change the document only together with the gateway allow-lists. See
// docs/unfinished-work-review-2026-10-08/EXPERIMENT_COMPATIBILITY.md (E5).
const (
	zoneThermalSpecDigest   = "sha256:d5907b52280fc34ec1d17c88e905c2ac79ad4358eb4bb2253e6f7efdf0f925fb"
	zoneThermalPolicyDigest = "sha256:e7e08b9fefb09299621e30217b62224441acbf9086f162c564e201f785aa140c"
)

func TestExperimentPolicyDocumentIsUnchanged(t *testing.T) {
	t.Parallel()
	digest, err := policy.DigestForVersion(zoneThermalSpecDigest)
	if err != nil {
		t.Fatal(err)
	}
	if digest != zoneThermalPolicyDigest {
		t.Errorf("policy digest for zone-thermal = %s; the policy document changed, so update the real-world-sensor gateway --device-policy-digest allow-lists in the same change, then this pin", digest)
	}
}
