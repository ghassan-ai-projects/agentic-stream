package domain_test

import (
	"bytes"
	"strings"
	"testing"
)

const enumIntent = `          entity_id:
            type: string
        additionalProperties: false
`

func specWithBaseline(baseline string) string {
	return editedSpec(enumIntent, `          entity_id:
            type: string
          level:
            type: string
            enum: [low, high]
          note:
            type: string
          tone:
            type: string
            enum: [soft, loud]
            pattern: "^s"
          mode:
            type: string
            enum: [hold, cool]
        additionalProperties: false
      modelWritableFields: [level, note, tone]
`+baseline)
}

func TestCompileAcceptsABaselineChoicePerDeclaredPhase(t *testing.T) {
	t.Parallel()
	compiled, err := compileSource(t, specWithBaseline("      baseline:\n        byPhase:\n          alert: {level: high}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := compiled.Actions.Intents[0].Baseline.ByPhase["alert"]["level"]; got != "high" {
		t.Fatalf("baseline choice = %q, want high", got)
	}
	if !bytes.Contains(compiled.CanonicalJSON, []byte(`"baseline":{"byPhase":{"alert":{"level":"high"}}}`)) {
		t.Fatalf("the baseline is not part of the canonical spec: %s", compiled.CanonicalJSON)
	}
}

func TestAnIntentWithoutABaselineCompilesToTheSameCanonicalJSON(t *testing.T) {
	t.Parallel()
	compiled, err := compileSource(t, minimalSpecYAML())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(compiled.CanonicalJSON, []byte("baseline")) {
		t.Fatalf("an intent without a baseline gained one in canonical JSON: %s", compiled.CanonicalJSON)
	}
}

func TestCompileRejectsABaselineChoiceTheSpecDoesNotDeclare(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, baseline, want string
	}{
		{"unknown phase", "          melting: {level: high}\n", `actions.intents.notify.baseline.byPhase.melting: unknown phase "melting"`},
		{"unknown parameter", "          alert: {volume: high}\n", `actions.intents.notify.baseline.byPhase.alert.volume: parameter "volume" is not declared by parameterSchema`},
		{"value outside the enum", "          alert: {level: extreme}\n", `actions.intents.notify.baseline.byPhase.alert.level: value "extreme" is not in the enum of parameter "level"`},
		{"parameter without an enum", "          alert: {note: high}\n", `value "high" is not in the enum of parameter "note"`},
		{"empty phase choices", "          alert: {}\n", "schema validation"},
		{"a number is read as text and must still be an enum value", "          alert: {level: 3}\n", `value "3" is not in the enum of parameter "level"`},
		{"non-scalar value", "          alert: {level: [high]}\n", "decode yaml"},
		{"a parameter the model may not write", "          alert: {mode: cool}\n", `actions.intents.notify.baseline.byPhase.alert.mode: parameter "mode" is not in modelWritableFields`},
		{"an enum value the parameter schema still refuses", "          alert: {tone: loud}\n", `actions.intents.notify.baseline.byPhase.alert.tone: value "loud" does not satisfy the schema of parameter "tone"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileSource(t, specWithBaseline("      baseline:\n        byPhase:\n"+tt.baseline))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("compile = %v, want %q", err, tt.want)
			}
		})
	}
}
