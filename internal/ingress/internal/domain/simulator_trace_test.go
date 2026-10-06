package domain

import (
	"strings"
	"testing"
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
	if err != nil || !isEvent || env.ID != "evt-1" || env.TenantID != DefaultTenant || env.Type != "pump.temperature.observed" {
		t.Fatalf("event %+v isEvent=%v err=%v", env, isEvent, err)
	}
	if err := acceptAll(t, trace, endLine); err != nil || trace.Finish() != nil || trace.Line() != 3 {
		t.Fatalf("finish err=%v line=%d", err, trace.Line())
	}
}

func TestSimulatorTraceRefusesGrammarViolations(t *testing.T) {
	t.Parallel()
	activation := `{"record_type":"model_activation","recorded_time":"2026-07-29T09:00:02Z","mode":"continue","model":{"schema_version":"0.1.0","id":"m","version":"1","entity_type":"pump","lateness_allowance":"5s","inputs":[],"windows":[],"facts":[],"states":[],"episode_types":[],"cognition_triggers":[],"budgets":{}}}`
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
	trace := NewSimulatorTrace(SimulatorOptions{})
	if err := trace.Finish(); err == nil {
		t.Fatal("an empty trace finished")
	}
	if err := acceptAll(t, trace, configLine); err != nil || trace.Finish() == nil {
		t.Fatalf("a trace without trace_end finished: %v", err)
	}
}

func TestSimulatorOptionsNormalizeDefaults(t *testing.T) {
	t.Parallel()
	got := SimulatorOptions{EntityType: "pump"}.Normalized()
	if got.TenantID != DefaultTenant || got.EventTypePrefix != "pump." || got.Source != "streams-simulator" {
		t.Fatalf("normalized = %+v", got)
	}
	if kept := (SimulatorOptions{TenantID: "t", EventTypePrefix: "x.", Source: "s"}).Normalized(); kept.TenantID != "t" || kept.EventTypePrefix != "x." || kept.Source != "s" {
		t.Fatalf("explicit options changed: %+v", kept)
	}
}
