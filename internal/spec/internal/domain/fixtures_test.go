package domain_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

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

func editedSpec(edits ...string) string {
	source := minimalSpecYAML()
	for i := 0; i+1 < len(edits); i += 2 {
		source = strings.Replace(source, edits[i], edits[i+1], 1)
	}
	return source
}

func compileSource(t *testing.T, source string) (*domain.CompiledSpec, error) {
	t.Helper()
	return domain.NewCompiler().CompileBytes(t.Context(), []byte(source), "test.yaml")
}

func compileFile(t *testing.T, path string) *domain.CompiledSpec {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := domain.NewCompiler().CompileBytes(t.Context(), data, path)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	return compiled
}

func mustFloat(t *testing.T, text string) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
