package spec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestCompilePredictiveMaintenance(t *testing.T) {
	compiled, err := spec.CompileFile(context.Background(), "../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}
	if compiled.Metadata.Name != "motor_bearing_degradation" {
		t.Fatalf("unexpected name %q", compiled.Metadata.Name)
	}
	if compiled.Digest == "" {
		t.Fatal("expected non-empty digest")
	}
	if len(compiled.CanonicalJSON) == 0 {
		t.Fatal("expected canonical JSON")
	}
}

func TestCompileRotatingMachinery(t *testing.T) {
	compiled, err := spec.CompileFile(context.Background(), "../../docs/design/examples/rotating-machinery.situation.yaml")
	if err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}
	if compiled.Metadata.Name != "pump_bearing_degradation" {
		t.Fatalf("unexpected name %q", compiled.Metadata.Name)
	}
	if len(compiled.Inputs) != 7 {
		t.Fatalf("input count = %d, want 7 simulator channels", len(compiled.Inputs))
	}
	if compiled.Digest == "" {
		t.Fatal("expected non-empty digest")
	}
}

func TestCompileZoneThermal(t *testing.T) {
	// The Real-World Sensor HIL-0 telemetry spec must compile against the
	// CURRENT schema grammar — it deliberately avoids the invented fields
	// (schema:, supervisor:, expectedFeedback:, aggregate: latest) that made the
	// round-1 starter fail. docs/plans/real-world-sensor-hil/01-telemetry-vertical.md
	compiled, err := spec.CompileFile(context.Background(), "../../docs/design/examples/zone-thermal.situation.yaml")
	if err != nil {
		t.Fatalf("CompileFile failed: %v", err)
	}
	if compiled.Metadata.Name != "zone_over_temp" {
		t.Fatalf("unexpected name %q", compiled.Metadata.Name)
	}
	if len(compiled.Inputs) != 5 {
		t.Fatalf("input count = %d, want 5 (temp, humidity, ambient, fan_tach, heartbeat)", len(compiled.Inputs))
	}
	if len(compiled.Actions.Intents) != 3 {
		t.Fatalf("intent count = %d, want 3 (install_watch_condition, set_indicator, select_thermal_mode)", len(compiled.Actions.Intents))
	}
	if compiled.Digest == "" {
		t.Fatal("expected non-empty digest")
	}
}

func TestCompileStableDigestForEquivalentYAML(t *testing.T) {
	yaml := minimalSpecYAML()

	c1, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "a.yaml")
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}

	yaml2 := strings.ReplaceAll(yaml, "  name: test\n  version: 0.1.0\n", "  version: 0.1.0\n  name: test\n")
	yaml2 = strings.ReplaceAll(yaml2, "  name: test\n", "  name:    test\n")

	c2, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml2), "b.yaml")
	if err != nil {
		t.Fatalf("second compile failed: %v", err)
	}
	if c1.Digest != c2.Digest {
		t.Fatalf("digests differ: %s vs %s", c1.Digest, c2.Digest)
	}
}

func TestCompilePromptContentChangesDigestWithoutVersionChange(t *testing.T) {
	base := minimalSpecYAML()
	changed := strings.Replace(base, "prompt: Analyze the situation and return a typed decision.", "prompt: Return a typed decision with explicit evidence.", 1)
	first, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(base), "base.yaml")
	if err != nil {
		t.Fatalf("compile base: %v", err)
	}
	second, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(changed), "changed.yaml")
	if err != nil {
		t.Fatalf("compile changed: %v", err)
	}
	if first.Digest == second.Digest {
		t.Fatal("prompt content change did not change compiled provenance")
	}
}

func TestEffectiveWatchConfidenceFloor(t *testing.T) {
	var explicitZero float64
	explicitCustom := 0.7
	tests := []struct {
		name    string
		actions spec.Actions
		want    float64
	}{
		{name: "omitted", want: 0.5},
		{name: "custom", actions: spec.Actions{WatchConfidenceFloor: &explicitCustom}, want: 0.7},
		{name: "opt out", actions: spec.Actions{WatchConfidenceFloor: &explicitZero}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.actions.EffectiveWatchConfidenceFloor(); got != tt.want {
				t.Fatalf("effective watch confidence floor = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompileWatchConfidenceFloorSchemaValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "accepted", value: "0.7"},
		{name: "negative", value: "-0.1", wantErr: true},
		{name: "over one", value: "1.1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := strings.Replace(
				minimalSpecYAML(),
				"actions:\n",
				"actions:\n  watch_confidence_floor: "+tt.value+"\n",
				1,
			)
			compiled, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "test.yaml")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected schema validation error")
				}
				return
			}
			if err != nil {
				t.Fatalf("compile failed: %v", err)
			}
			if compiled.Actions.WatchConfidenceFloor == nil || *compiled.Actions.WatchConfidenceFloor != 0.7 {
				t.Fatalf("compiled watch confidence floor = %v, want 0.7", compiled.Actions.WatchConfidenceFloor)
			}
		})
	}
}

func TestCompileRejectsUndeclaredPayloadField(t *testing.T) {
	yaml := strings.Replace(minimalSpecYAML(), "field: data.value", "field: data.not_declared", 1)
	_, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "test.yaml")
	if err == nil || !strings.Contains(err.Error(), "not_declared") {
		t.Fatalf("expected undeclared payload field diagnostic, got %v", err)
	}
}

func TestCompileRejectsPayloadUnitMismatch(t *testing.T) {
	yaml := strings.Replace(minimalSpecYAML(), "    aggregate: mean\n", "    aggregate: mean\n    unit: kelvin\n", 1)
	_, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "test.yaml")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected unit mismatch diagnostic, got %v", err)
	}
}

func TestCompileAcceptsLatestAggregate(t *testing.T) {
	yaml := strings.Replace(minimalSpecYAML(), "    aggregate: mean\n", "    aggregate: latest\n", 1)
	if _, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "latest.yaml"); err != nil {
		t.Fatalf("compile latest aggregate: %v", err)
	}
}

func TestCompileRejectsUnknownOperatorOutput(t *testing.T) {
	yaml := strings.ReplaceAll(minimalSpecYAML(),
		"      input: mean_value",
		"      input: unknown_output")
	_, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "test.yaml")
	if err == nil {
		t.Fatal("expected error for unknown operator output")
	}
	if !strings.Contains(err.Error(), "unknown operator output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileRejectsInvalidCEL(t *testing.T) {
	yaml := strings.ReplaceAll(minimalSpecYAML(),
		"      when: features.mean_value > 1.0",
		"      when: features.mean_value >")
	_, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "test.yaml")
	if err == nil {
		t.Fatal("expected error for invalid CEL")
	}
	if !strings.Contains(err.Error(), "cel:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileRejectsDuplicateYAMLKey(t *testing.T) {
	yaml := strings.ReplaceAll(minimalSpecYAML(),
		"  name: test\n",
		"  name: test\n  name: test2\n")
	_, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "test.yaml")
	if err == nil {
		t.Fatal("expected error for duplicate YAML key")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileRejectsDuplicateWindowName(t *testing.T) {
	yaml := strings.ReplaceAll(minimalSpecYAML(),
		"  - name: w1\n    kind: tumbling\n    size: 1m",
		"  - name: w1\n    kind: tumbling\n    size: 1m\n  - name: w1\n    kind: tumbling\n    size: 2m")
	_, err := spec.NewCompiler().CompileBytes(context.Background(), []byte(yaml), "test.yaml")
	if err == nil {
		t.Fatal("expected error for duplicate window name")
	}
	if !strings.Contains(err.Error(), "duplicate window name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func minimalSpecYAML() string {
	return `apiVersion: agentic-stream/v1
kind: SituationSpec
metadata:
  name: test
  version: 0.1.0
inputs:
  - name: temp
    eventType: sensor.temperature
    schemaVersion: "1.0"
    schema: sensor.temperature/1.0
    partitionKey: entity.id
    entityType: sensor
time:
  watermarkStrategy: bounded_out_of_orderness
  maxOutOfOrderness: 1m
  idleTimeout: 5m
  allowedLateness: 10m
  latePolicy: drop_with_audit
windows:
  - name: w1
    kind: tumbling
    size: 1m
operators:
  - name: op1
    kind: aggregate
    inputs: [temp]
    field: data.value
    window: w1
    aggregate: mean
    output: mean_value
situation:
  type: simple
  entityKey: entity.id
  occurrence:
    openWhen: features.mean_value > 1.0
    closeWhen: features.mean_value <= 1.0
  initialPhase: ok
  phases:
    - name: ok
      severity: 0
    - name: alert
      severity: 50
  transitions:
    - from: ok
      to: alert
      when: features.mean_value > 1.0
  reducers:
    - field: facts.mean
      strategy: latest_event_time
      input: mean_value
cognition:
  triggers:
    - name: diagnose
      when: situation.phase == "alert"
      score: 1.0
      threshold: 0.5
      lane: fast
      materialDelta: delta.phase_changed
  executor:
    name: fake
    objective: test
    prompt: Analyze the situation and return a typed decision.
    modelPolicy: fake
    promptVersion: v1
    decisionSchema: schemas/test.json
    tools: []
    budget:
      wallTime: 1m
      modelCalls: 1
      inputTokens: 1
      outputTokens: 1
      toolCalls: 0
      toolResultBytes: 0
      totalToolResultBytes: 0
      providerRetries: 0
      costMicrounits: 0
actions:
  intents:
    - type: notify
      risk: R0
      parameterSchema:
        type: object
        properties:
          entity_id:
            type: string
        additionalProperties: false
`
}
