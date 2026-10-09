package policy_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// Every device command carries the policy digest, and the real-world-sensor
// bench gateway allow-lists it (arduino_gateway.py --device-policy-digest).
// The digest covers the policy document bound to a spec digest, so changing
// the document's rules changes the value every deployed gateway accepts.
// Change the document only together with the gateway allow-lists.
// experimentPolicyDigests map each experiment spec digest (pinned by the spec
// module) to the policy digest its device commands carry.
var experimentPolicyDigests = []struct{ name, specDigest, policyDigest string }{
	{"zone-thermal", "sha256:d5907b52280fc34ec1d17c88e905c2ac79ad4358eb4bb2253e6f7efdf0f925fb", "sha256:e7e08b9fefb09299621e30217b62224441acbf9086f162c564e201f785aa140c"},
	{"zone-thermal-sim", "sha256:a6153efe15c9a2b5ea7706d8e8f62312263b8c994eb6839caeb45b84412ae171", "sha256:7a7f450651fb186c2b96939bb52c4b95a8c644ced69d244e156b17df022ade56"},
	{"zone-thermal-bench", "sha256:b1b60be7ddae1ef91da2b43f52ee44263f6d2a367bfdd0605bdd1c28ad9c34cf", "sha256:d973fc8a866d49afaa36944cd8308221a229e88db04b6330917192dbd18a5a6f"},
}

func TestExperimentPolicyDocumentIsUnchanged(t *testing.T) {
	t.Parallel()
	for _, pinned := range experimentPolicyDigests {
		digest, err := policy.DigestForVersion(pinned.specDigest)
		if err != nil {
			t.Fatal(err)
		}
		if digest != pinned.policyDigest {
			t.Errorf("policy digest for %s = %s; the policy document changed, so update the real-world-sensor gateway --device-policy-digest allow-lists in the same change, then this pin", pinned.name, digest)
		}
	}
}
