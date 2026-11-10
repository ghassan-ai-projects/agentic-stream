package domain_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

const tempHeartbeat = `  - name: temp_missing
    kind: missing_heartbeat
    inputs: [temp]
    duration: 5m
    output: temp_missing_5m
`

func TestAnAdvisoryInputIsCompiledWithTheHeartbeatThatJudgesItsLoss(t *testing.T) {
	t.Parallel()
	compiled, err := compileSource(t, editedSpec("    entityType: sensor\n", "    entityType: sensor\n    completenessRole: advisory\n",
		"    output: mean_value\n", "    output: mean_value\n"+tempHeartbeat))
	if err != nil {
		t.Fatal(err)
	}
	input, advisory := compiled.AdvisoryHeartbeatInput("temp_missing_5m")
	if !slices.Equal(compiled.AdvisoryInputs(), []string{"temp"}) || !advisory || input != "temp" {
		t.Fatalf("advisory inputs %v, heartbeat input %q %v: want temp, judged by temp_missing", compiled.AdvisoryInputs(), input, advisory)
	}
	if _, isHeartbeat := compiled.AdvisoryHeartbeatInput("mean_value"); isHeartbeat {
		t.Fatal("mean_value is no advisory heartbeat")
	}
	if domain.InputStaleFact("temp") != "input_stale.temp" {
		t.Fatalf("stale fact = %q", domain.InputStaleFact("temp"))
	}
}

func TestAnAdvisoryInputWithoutAHeartbeatOrAnUnknownRoleIsRejected(t *testing.T) {
	t.Parallel()
	_, err := compileSource(t, editedSpec("    entityType: sensor\n", "    entityType: sensor\n    completenessRole: advisory\n"))
	var compileErr *domain.CompileError
	if !errors.As(err, &compileErr) || compileErr.Path != "inputs.temp.completenessRole" {
		t.Fatalf("err = %v, want a compile error at inputs.temp.completenessRole: nothing would notice the loss", err)
	}
	if _, err := compileSource(t, editedSpec("    entityType: sensor\n", "    entityType: sensor\n    completenessRole: optional\n")); err == nil ||
		!strings.Contains(err.Error(), "completenessRole") {
		t.Fatalf("err = %v, want the schema to reject an unknown role", err)
	}
}

func TestARequiredRoleAndNoRoleCompileAlikeExceptForTheDeclaredField(t *testing.T) {
	t.Parallel()
	absent, err := compileSource(t, minimalSpecYAML())
	if err != nil {
		t.Fatal(err)
	}
	required, err := compileSource(t, editedSpec("    entityType: sensor\n", "    entityType: sensor\n    completenessRole: required\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(absent.AdvisoryInputs()) != 0 || len(required.AdvisoryInputs()) != 0 || strings.Contains(string(absent.CanonicalJSON), "completenessRole") {
		t.Fatalf("absent %v, required %v advisory inputs: want none, and no completenessRole in a spec that declares none", absent.AdvisoryInputs(), required.AdvisoryInputs())
	}
}
