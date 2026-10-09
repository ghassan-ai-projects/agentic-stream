package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const (
	configLine = `{"record_type":"runtime_config","runtime_version":"1","storage_schema_version":1,"max_episodes_per_hour":10}`
	endLine    = `{"record_type":"trace_end","until":"2026-07-29T10:00:00Z"}`
)

func eventLine(id, arrival string) string {
	return `{"record_type":"event","event":{"id":"` + id + `","entity_type":"pump","entity_id":"p1","type":"temperature","event_time":"2026-07-29T09:00:00Z","arrival_time":"` + arrival + `","value":26.5}}`
}

func acceptAll(t *testing.T, trace *SimulatorTrace, lines ...string) error {
	t.Helper()
	for _, line := range lines {
		trace.NextLine()
		if _, _, err := trace.Accept([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

func TestSimulatorTraceAcceptsAWellFormedTrace(t *testing.T) {
	t.Parallel()
	trace := NewSimulatorTrace(SimulatorOptions{EntityType: "pump"})
	trace.NextLine()
	if _, isEvent, err := trace.Accept([]byte(configLine)); err != nil || isEvent {
		t.Fatalf("config isEvent=%v err=%v", isEvent, err)
	}
	trace.NextLine()
	env, isEvent, err := trace.Accept([]byte(eventLine("evt-1", "2026-07-29T09:00:01Z")))
	if err != nil || !isEvent || env.ID != "evt-1" || env.TenantID != contractsv1.TenantID || env.Type != "pump.temperature.observed" {
		t.Fatalf("event %+v isEvent=%v err=%v", env, isEvent, err)
	}
	if err := acceptAll(t, trace, endLine); err != nil || trace.Finish() != nil || trace.Line() != 3 {
		t.Fatalf("finish err=%v line=%d", err, trace.Line())
	}
}

func TestSimulatorTraceRefusesGrammarViolations(t *testing.T) {
	t.Parallel()
	activation := activationLine(t, nil)
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{"config must be first", []string{eventLine("e", "2026-07-29T09:00:01Z")}, "runtime_config must be first"},
		{"duplicate config", []string{configLine, configLine}, "duplicate runtime_config"},
		{"record after end", []string{configLine, endLine, eventLine("e", "2026-07-29T09:00:01Z")}, "follows trace_end"},
		{"unknown record", []string{configLine, `{"record_type":"banner"}`}, `unknown simulator record_type "banner"`},
		{"unparseable record", []string{configLine, "{"}, "parse simulator line 2"},
		{"recorded time must increase", []string{configLine, eventLine("a", "2026-07-29T09:00:05Z"), eventLine("b", "2026-07-29T09:00:05Z")}, "strictly increasing"},
		{"activation must be later", []string{configLine, eventLine("a", "2026-07-29T09:00:05Z"), activation}, "strictly increasing"},
		{"end must follow every input", []string{configLine, eventLine("a", "2026-07-29T10:30:00Z"), endLine}, "later than every recorded input"},
		{"event needs arrival", []string{configLine, `{"record_type":"event","event":{"id":"e","entity_type":"pump","entity_id":"p1","type":"temperature","event_time":"2026-07-29T09:00:00Z"}}`}, "arrival_time is required"},
		{"incomplete config", []string{`{"record_type":"runtime_config","runtime_version":"","storage_schema_version":1,"max_episodes_per_hour":10}`}, "incomplete runtime_config"},
		{"config unknown field", []string{`{"record_type":"runtime_config","runtime_version":"1","storage_schema_version":1,"max_episodes_per_hour":10,"x":1}`}, "unknown fields"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			trace := NewSimulatorTrace(SimulatorOptions{})
			if err := acceptAll(t, trace, tc.lines...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFinishRequiresFramingRecords(t *testing.T) {
	t.Parallel()
	const want = "runtime_config first and trace_end last"
	trace := NewSimulatorTrace(SimulatorOptions{})
	if err := trace.Finish(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("an empty trace: err = %v, want %q", err, want)
	}
	if err := acceptAll(t, trace, configLine); err != nil {
		t.Fatal(err)
	}
	if err := trace.Finish(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("a trace without trace_end: err = %v, want %q", err, want)
	}
}

func TestSimulatorTraceAcceptsAModelActivationBetweenEvents(t *testing.T) {
	t.Parallel()
	trace := NewSimulatorTrace(SimulatorOptions{EntityType: "pump"})
	lines := []string{configLine, eventLine("a", "2026-07-29T09:00:01Z"), activationLine(t, nil), eventLine("b", "2026-07-29T09:00:03Z"), endLine}
	if err := acceptAll(t, trace, lines...); err != nil || trace.Finish() != nil {
		t.Fatalf("a well-formed trace with an activation: err = %v", err)
	}
}

func TestSimulatorTraceRefusesInvalidControlRecords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want string
	}{
		{"activation with an extra field", activationLine(t, func(r map[string]any) { r["extra"] = 1 }), `contains unknown field "extra"`},
		{"activation without a recorded time", activationLine(t, func(r map[string]any) { delete(r, "recorded_time") }), "model_activation is incomplete"},
		{"activation with an unparseable recorded time", activationLine(t, func(r map[string]any) { r["recorded_time"] = "soon" }), "parse recorded_time"},
		{"activation mode outside continue and reset", activationLine(t, func(r map[string]any) { r["mode"] = "pause" }), "mode must be continue or reset"},
		{"activation without a model object", activationLine(t, func(r map[string]any) { r["model"] = "m" }), "model is required"},
		{"activation model without an id", activationLine(t, func(r map[string]any) { delete(r["model"].(map[string]any), "id") }), "model.id is required"},
		{"activation model of another schema version", activationLine(t, func(r map[string]any) { r["model"].(map[string]any)["schema_version"] = "0.2.0" }), "model.schema_version must be 0.1.0"},
		{"config below storage schema version one", `{"record_type":"runtime_config","runtime_version":"1","storage_schema_version":0,"max_episodes_per_hour":10}`, "incomplete runtime_config"},
		{"config above the episode cap", `{"record_type":"runtime_config","runtime_version":"1","storage_schema_version":1,"max_episodes_per_hour":10001}`, "incomplete runtime_config"},
		{"config with a fractional episode cap", `{"record_type":"runtime_config","runtime_version":"1","storage_schema_version":1,"max_episodes_per_hour":1.5}`, "incomplete runtime_config"},
		{"trace end with an extra field", `{"record_type":"trace_end","until":"2026-07-29T10:00:00Z","extra":1}`, "trace_end contains unknown fields"},
		{"trace end with an unparseable until", `{"record_type":"trace_end","until":"later"}`, "parse until"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			trace := NewSimulatorTrace(SimulatorOptions{})
			lines := []string{configLine, tc.line}
			if strings.Contains(tc.line, `"record_type":"runtime_config"`) {
				lines = []string{tc.line}
			}
			if err := acceptAll(t, trace, lines...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSimulatorOptionsNormalizeDefaults(t *testing.T) {
	t.Parallel()
	got := NewSimulatorTrace(SimulatorOptions{EntityType: "pump"}).Options()
	if got.TenantID != contractsv1.TenantID || got.EventTypePrefix != "pump." || got.Source != "streams-simulator" {
		t.Fatalf("normalized = %+v", got)
	}
	if kept := (SimulatorOptions{TenantID: "t", EventTypePrefix: "x.", Source: "s"}).Normalized(); kept.TenantID != "t" || kept.EventTypePrefix != "x." || kept.Source != "s" {
		t.Fatalf("explicit options changed: %+v", kept)
	}
}

func activationLine(t *testing.T, edit func(record map[string]any)) string {
	t.Helper()
	record := map[string]any{
		"record_type": "model_activation", "recorded_time": "2026-07-29T09:00:02Z", "mode": "continue",
		"model": map[string]any{
			"schema_version": "0.1.0", "id": "m", "version": "1", "entity_type": "pump", "lateness_allowance": "5s",
			"inputs": []any{}, "windows": []any{}, "facts": []any{}, "states": []any{}, "episode_types": []any{},
			"cognition_triggers": []any{}, "budgets": map[string]any{},
		},
	}
	if edit != nil {
		edit(record)
	}
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return string(line)
}
