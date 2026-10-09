package domain_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestExpressionThatCannotBeEvaluatedFailsTheFeature(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*spec.CompiledSpec)
		value   float64
		wantErr string
	}{
		{"syntax error in the open condition", func(c *spec.CompiledSpec) { c.Situation.Occurrence.OpenWhen = "features.vibration_rms >" }, 4.0, "compile cel"},
		{"open condition that is not boolean", func(c *spec.CompiledSpec) { c.Situation.Occurrence.OpenWhen = "features.vibration_rms + 1.0" }, 4.0, "evaluate situation"},
		{"unknown function in a transition", func(c *spec.CompiledSpec) { c.Situation.Transitions[0].When = "nothing(features)" }, 5.0, "compile cel"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			compiled := vibrationSpec()
			tc.mutate(compiled)
			_, err := newEngine(t, compiled).ApplyFeature(t.Context(), vibration(tc.value, base), base)
			requireErrorContaining(t, err, tc.wantErr)
		})
	}
}

func TestEmptyConditionsNeverHold(t *testing.T) {
	t.Parallel()
	compiled := vibrationSpec()
	compiled.Situation.Occurrence = spec.Occurrence{}
	compiled.Situation.Transitions = nil
	if versions := apply(t, newEngine(t, compiled), vibration(9.0, base)); len(versions) != 0 {
		t.Fatalf("published %+v without any open condition", versions)
	}
}

func TestSpecWithoutAValidDigestCannotPublishAVersion(t *testing.T) {
	t.Parallel()
	for digest, wantErr := range map[string]string{
		"":              "compiled spec has no digest",
		"sha256:nothex": "invalid spec digest",
	} {
		t.Run("digest="+digest, func(t *testing.T) {
			t.Parallel()
			compiled := vibrationSpec()
			compiled.Digest = digest
			_, err := newEngine(t, compiled).ApplyFeature(t.Context(), vibration(5.0, base), base)
			requireErrorContaining(t, err, wantErr)
		})
	}
}
