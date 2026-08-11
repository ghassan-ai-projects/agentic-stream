package reference_test

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	modelSchemaID = "https://agentic-stream.local/schemas/situation-model-v0.1.schema.json"
	eventSchemaID = "https://agentic-stream.local/schemas/event-envelope-v0.1.schema.json"
	traceSchemaID = "https://agentic-stream.local/schemas/trace-record-v0.1.schema.json"
)

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema load denied: %s", url)
}

func loadJSON(t *testing.T, path string) any {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	value, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func compiler(t *testing.T) *jsonschema.Compiler {
	t.Helper()

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})

	resources := []struct {
		id   string
		path string
	}{
		{modelSchemaID, "../contracts/situation-model-v0.1.schema.json"},
		{eventSchemaID, "../contracts/event-envelope-v0.1.schema.json"},
		{traceSchemaID, "../contracts/trace-record-v0.1.schema.json"},
	}
	for _, resource := range resources {
		if err := compiler.AddResource(resource.id, loadJSON(t, resource.path)); err != nil {
			t.Fatal(err)
		}
	}
	return compiler
}

func TestExampleModelsValidate(t *testing.T) {
	schema, err := compiler(t).Compile(modelSchemaID)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"../examples/motor-warning.situation-model.json",
		"../examples/coldroom-integrity.situation-model.json",
	} {
		t.Run(path, func(t *testing.T) {
			if err := schema.Validate(loadJSON(t, path)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTraceSchemaUsesOnlyPreloadedReferences(t *testing.T) {
	schema, err := compiler(t).Compile(traceSchemaID)
	if err != nil {
		t.Fatal(err)
	}

	records := []any{
		map[string]any{
			"record_type":            "runtime_config",
			"runtime_version":        "situation-model-v0.1",
			"storage_schema_version": 1,
			"max_episodes_per_hour":  100,
		},
		map[string]any{
			"record_type":   "model_activation",
			"recorded_time": "2026-07-29T08:15:29Z",
			"mode":          "continue",
			"model":         loadJSON(t, "../examples/motor-warning.situation-model.json"),
		},
		map[string]any{
			"record_type": "event",
			"event": map[string]any{
				"id":           "evt-1",
				"entity_type":  "motor",
				"entity_id":    "motor-17",
				"type":         "motor.vibration",
				"event_time":   "2026-07-29T08:15:30Z",
				"arrival_time": "2026-07-29T08:15:31Z",
				"value":        5.2,
				"unit":         "mm/s",
			},
		},
		map[string]any{
			"record_type": "trace_end",
			"until":       "2026-07-29T09:00:00Z",
		},
	}

	for index, record := range records {
		if err := schema.Validate(record); err != nil {
			t.Fatalf("record %d: %v", index, err)
		}
	}
}

func TestSituationModelRequiresExplicitLatenessAndBudgets(t *testing.T) {
	schema, err := compiler(t).Compile(modelSchemaID)
	if err != nil {
		t.Fatal(err)
	}

	model := loadJSON(t, "../examples/motor-warning.situation-model.json").(map[string]any)
	delete(model, "lateness_allowance")
	delete(model, "budgets")

	if err := schema.Validate(model); err == nil {
		t.Fatal(errors.New("model without explicit lateness and budgets unexpectedly validated"))
	}
}
