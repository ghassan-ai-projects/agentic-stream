package main

import (
	"path/filepath"
	"strings"
	"testing"
)

var digestsWithoutACompletenessRole = map[string][2]string{
	"predictive-maintenance/predictive-maintenance.situation.yaml": {"sha256:4d039de7873c4faaaf6429aeaefc3d27cd797b099db551952637801d07040581", "sha256:c798f4986f8b000cabda3c86ec10d147361885c4729a413a916a42ae43982104"},
	"pump-station/pump-station.situation.yaml":                     {"sha256:f997e7eb22738bbcf86a51e109b6f8c5930de9f976127f15c6c9bfc6addb2296", "sha256:c56f537863156a3e42b18d82bdc2652fd7a9ca1e7097911839436ef269986454"},
	"real-world-sensor/zone-thermal-bench.situation.yaml":          {"sha256:b1b60be7ddae1ef91da2b43f52ee44263f6d2a367bfdd0605bdd1c28ad9c34cf", "sha256:d973fc8a866d49afaa36944cd8308221a229e88db04b6330917192dbd18a5a6f"},
	"real-world-sensor/zone-thermal-sim.situation.yaml":            {"sha256:a6153efe15c9a2b5ea7706d8e8f62312263b8c994eb6839caeb45b84412ae171", "sha256:7a7f450651fb186c2b96939bb52c4b95a8c644ced69d244e156b17df022ade56"},
	"rotating-machinery/rotating-machinery.situation.yaml":         {"sha256:43b6b00059949a82488f519a79a443ef8db03ee14e7c76a47fd5a0cef6c73c70", "sha256:0e20778e4f3e360ffe9d65ba2c719af809b87bddca948924032311c1f75bc92d"},
	"thermal-chamber/zone-thermal.situation.yaml":                  {"sha256:d5907b52280fc34ec1d17c88e905c2ac79ad4358eb4bb2253e6f7efdf0f925fb", "sha256:e7e08b9fefb09299621e30217b62224441acbf9086f162c564e201f785aa140c"},
	"belt-conveyor/testdata/line-overload.situation.yaml":          {"sha256:f799641c655f82a7b8d8f97ef0c3840546d10fb4a38e22b503e2d4f533a73e45", "sha256:c25aad84626829bed7b1005cd061cce56756045836210223282116d65c91e1de"},
	"belt-conveyor/testdata/drive-degradation-planned-stop.situation.yaml": {"sha256:20cce9c6d2cf7fe1e1ab05d87517702dda388ea9b0b68647c52b29d80af51f85",
		"sha256:f35e0b903760beded0cd93d439be9451275667c5e33fc4c41941fc54be2fb03a"},
	"belt-conveyor/testdata/master-loss.situation.yaml": {"sha256:b459dd5917ca8f3d9cc5f449fc89a34c946bf77b2533730c1d0efab8c70b3e42", "sha256:94a9bcef7effac73d049a8803f1121c49fd9956bf8ffec4897f4a9dbe6fca898"},
}

func TestASpecThatDeclaresNoCompletenessRoleKeepsTheSpecAndPolicyDigestsItHadBeforeTheField(t *testing.T) {
	t.Parallel()
	for path, want := range digestsWithoutACompletenessRole {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			args := []string{"validate", filepath.Join("..", "..", "examples", path)}
			if strings.HasPrefix(path, "belt-conveyor/") {
				args = append(args, "--event-schemas", conveyorEventSchemas)
			}
			out := runCLI(t, newRootCommand(), args...)
			if !strings.Contains(out, "\ndigest: "+want[0]+"\n") || !strings.Contains(out, "\npolicy_digest: "+want[1]+"\n") {
				t.Fatalf("validate %s = %q, want digest %s and policy digest %s", path, out, want[0], want[1])
			}
		})
	}
}
