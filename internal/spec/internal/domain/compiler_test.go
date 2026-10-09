package domain_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

func TestCompileSealsTheExampleSpecs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		dir     string
		file    string
		name    string
		inputs  int
		intents int
	}{
		{dir: "predictive-maintenance", file: "predictive-maintenance", name: "motor_bearing_degradation", inputs: -1, intents: -1},
		{dir: "rotating-machinery", file: "rotating-machinery", name: "pump_bearing_degradation", inputs: 7, intents: -1},
		{dir: "thermal-chamber", file: "zone-thermal", name: "zone_over_temp", inputs: 5, intents: 3},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			compiled := compileFile(t, "../../../../examples/"+tt.dir+"/"+tt.file+".situation.yaml")
			if compiled.Metadata.Name != tt.name {
				t.Fatalf("name = %q, want %q", compiled.Metadata.Name, tt.name)
			}
			if tt.inputs >= 0 && len(compiled.Inputs) != tt.inputs {
				t.Fatalf("input count = %d, want %d", len(compiled.Inputs), tt.inputs)
			}
			if tt.intents >= 0 && len(compiled.Actions.Intents) != tt.intents {
				t.Fatalf("intent count = %d, want %d", len(compiled.Actions.Intents), tt.intents)
			}
			if !strings.HasPrefix(compiled.Digest, "sha256:") || len(compiled.CanonicalJSON) == 0 {
				t.Fatalf("spec was not sealed: digest %q, %d canonical bytes", compiled.Digest, len(compiled.CanonicalJSON))
			}
		})
	}
}

func TestCompileGivesEquivalentYAMLTheSameDigest(t *testing.T) {
	t.Parallel()
	reordered := editedSpec("  name: test\n  version: 0.1.0\n", "  version: 0.1.0\n  name:    test\n")

	first, err := compileSource(t, minimalSpecYAML())
	if err != nil {
		t.Fatal(err)
	}
	second, err := compileSource(t, reordered)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("digests differ: %s vs %s", first.Digest, second.Digest)
	}
}

func TestCompilePromptContentChangesTheDigestWithoutAVersionChange(t *testing.T) {
	t.Parallel()
	changed := editedSpec("prompt: Analyze the situation and return a typed decision.", "prompt: Return a typed decision with explicit evidence.")

	first, err := compileSource(t, minimalSpecYAML())
	if err != nil {
		t.Fatal(err)
	}
	second, err := compileSource(t, changed)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatalf("a prompt change kept digest %s", first.Digest)
	}
}

func TestConcurrentCompilesAgreeOnEverySource(t *testing.T) {
	t.Parallel()
	valid := minimalSpecYAML()
	invalid := editedSpec("aggregate: mean", "aggregate: unsupported")
	want, err := compileSource(t, valid)
	if err != nil {
		t.Fatal(err)
	}

	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := compileSource(t, valid)
			if err != nil || got.Digest != want.Digest {
				t.Errorf("valid spec: digest %v, err %v, want digest %s", got, err, want.Digest)
			}
			if _, err := compileSource(t, invalid); err == nil || !strings.Contains(err.Error(), "schema validation") {
				t.Errorf("invalid spec: err = %v, want a schema validation failure", err)
			}
		}()
	}
	group.Wait()
}

func TestEffectiveWatchConfidenceFloor(t *testing.T) {
	t.Parallel()
	var explicitZero float64
	explicitCustom := 0.7
	tests := []struct {
		name    string
		actions domain.Actions
		want    float64
	}{
		{name: "omitted", want: 0.5},
		{name: "custom", actions: domain.Actions{WatchConfidenceFloor: &explicitCustom}, want: 0.7},
		{name: "opt out", actions: domain.Actions{WatchConfidenceFloor: &explicitZero}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.actions.EffectiveWatchConfidenceFloor(); got != tt.want {
				t.Fatalf("effective watch confidence floor = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompileBoundsTheWatchConfidenceFloorToZeroAndOne(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value   string
		wantErr bool
	}{
		{value: "0"},
		{value: "0.7"},
		{value: "1"},
		{value: "-0.1", wantErr: true},
		{value: "1.1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			compiled, err := compileSource(t, editedSpec("actions:\n", "actions:\n  watch_confidence_floor: "+tt.value+"\n"))
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "schema validation") {
					t.Fatalf("floor %s: err = %v, want a schema validation failure", tt.value, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := compiled.Actions.WatchConfidenceFloor; got == nil || *got != mustFloat(t, tt.value) {
				t.Fatalf("compiled floor = %v, want %s", got, tt.value)
			}
		})
	}
}

func TestCompileAcceptsTheLatestAggregate(t *testing.T) {
	t.Parallel()
	compiled, err := compileSource(t, editedSpec("    aggregate: mean\n", "    aggregate: latest\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := compiled.Operators[0].Aggregate; got != "latest" {
		t.Fatalf("aggregate = %q, want latest", got)
	}
}

func TestCompileAcceptsDigestPinnedExecutorSkills(t *testing.T) {
	t.Parallel()
	const tree = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	compiled, err := compileSource(t, editedSpec("    tools: []\n", "    tools: []\n    skills:\n      - name: diagnostic_playbook\n        tree_sha256: "+tree+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	skills := compiled.Cognition.Executor.Skills
	if len(skills) != 1 || skills[0].Name != "diagnostic_playbook" || skills[0].TreeSHA256 != tree {
		t.Fatalf("compiled skills = %+v, want diagnostic_playbook pinned to %s", skills, tree)
	}
}

func TestCompileRejectsASpecThatBreaksARule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name:    "payload field the schema does not declare",
			source:  editedSpec("field: data.value", "field: data.not_declared"),
			wantErr: `operators.op1.field: payload field "not_declared" is not declared by schema "sensor.temperature/1.0"`,
		},
		{
			name:    "aggregate unit that differs from the payload field",
			source:  editedSpec("    aggregate: mean\n", "    aggregate: mean\n    unit: kelvin\n"),
			wantErr: `operators.op1.unit: unit "kelvin" does not match field "value" unit "celsius"`,
		},
		{
			name:    "count window",
			source:  editedSpec("    kind: tumbling\n    size: 1m", "    kind: count\n    count: 5"),
			wantErr: "schema validation",
		},
		{
			name:    "map operator",
			source:  editedSpec("    kind: aggregate\n", "    kind: map\n"),
			wantErr: "schema validation",
		},
		{
			name:    "variance aggregate",
			source:  editedSpec("    aggregate: mean\n", "    aggregate: variance\n"),
			wantErr: "schema validation",
		},
		{
			name:    "max reducer",
			source:  editedSpec("      strategy: latest_event_time\n", "      strategy: max\n"),
			wantErr: "schema validation",
		},
		{
			name:    "retention control nothing enforces",
			source:  minimalSpecYAML() + "retention:\n  rawEvents: 7d\n",
			wantErr: "retention",
		},
		{
			name:    "telemetry control nothing enforces",
			source:  minimalSpecYAML() + "telemetry:\n  traceSampleRatio: 1\n",
			wantErr: "telemetry",
		},
		{
			name:    "duplicate yaml key",
			source:  editedSpec("  name: test\n", "  name: test\n  name: test2\n"),
			wantErr: `duplicate key "name"`,
		},
		{
			name:    "malformed yaml",
			source:  "inputs: [unclosed",
			wantErr: "parse yaml",
		},
		{
			name:    "duplicate input name",
			source:  editedSpec("time:\n", "  - name: temp\n    eventType: sensor.temperature\n    schemaVersion: \"1.0\"\n    schema: sensor.temperature/1.0\n    partitionKey: entity.id\n    entityType: sensor\ntime:\n"),
			wantErr: `inputs: duplicate input name "temp"`,
		},
		{
			name:    "duplicate window name",
			source:  editedSpec("  - name: w1\n    kind: tumbling\n    size: 1m", "  - name: w1\n    kind: tumbling\n    size: 1m\n  - name: w1\n    kind: tumbling\n    size: 2m"),
			wantErr: `duplicate window name "w1"`,
		},
		{
			name:    "duplicate operator name",
			source:  editedSpec("    output: mean_value\n", "    output: mean_value\n  - name: op1\n    kind: aggregate\n    inputs: [temp]\n    field: data.value\n    window: w1\n    aggregate: mean\n    output: other_value\n"),
			wantErr: `operators: duplicate operator name "op1"`,
		},
		{
			name:    "duplicate operator output",
			source:  editedSpec("    output: mean_value\n", "    output: mean_value\n  - name: op2\n    kind: aggregate\n    inputs: [temp]\n    field: data.value\n    window: w1\n    aggregate: mean\n    output: mean_value\n"),
			wantErr: `operators: duplicate operator output "mean_value"`,
		},
		{
			name:    "duplicate phase name",
			source:  editedSpec("    - name: alert\n", "    - name: ok\n      severity: 0\n    - name: alert\n"),
			wantErr: `situation.phases: duplicate phase name "ok"`,
		},
		{
			name:    "event schema nobody registered",
			source:  editedSpec("schema: sensor.temperature/1.0", "schema: sensor.unregistered/1.0"),
			wantErr: `inputs.temp.schema: unknown event schema "sensor.unregistered/1.0"`,
		},
		{
			name:    "event schema bound to another event type",
			source:  editedSpec("eventType: sensor.temperature", "eventType: sensor.other"),
			wantErr: "inputs.temp.schema: schema reference does not match eventType/schemaVersion",
		},
		{
			name:    "operator input that is neither an input nor an output",
			source:  editedSpec("    inputs: [temp]\n", "    inputs: [nowhere]\n"),
			wantErr: `operators.op1.inputs: unknown input "nowhere"`,
		},
		{
			name:    "operator window nobody declared",
			source:  editedSpec("    window: w1\n", "    window: nowhere\n"),
			wantErr: `operators.op1.window: unknown window "nowhere"`,
		},
		{
			name:    "reducer over an unknown operator output",
			source:  editedSpec("      input: mean_value", "      input: unknown_output"),
			wantErr: `situation.reducers.facts.mean.input: unknown operator output "unknown_output"`,
		},
		{
			name:    "initial phase nobody declared",
			source:  editedSpec("initialPhase: ok", "initialPhase: missing"),
			wantErr: `situation.initialPhase: unknown phase "missing"`,
		},
		{
			name:    "transition from an unknown phase",
			source:  editedSpec("    - from: ok\n", "    - from: missing\n"),
			wantErr: `situation.transitions.missing-alert.from: unknown phase "missing"`,
		},
		{
			name:    "transition to an unknown phase",
			source:  editedSpec("      to: alert\n", "      to: missing\n"),
			wantErr: `situation.transitions.ok-missing.to: unknown phase "missing"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			compiled, err := compileSource(t, tt.source)
			if compiled != nil || err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("compiled = %v, err = %v, want a failure containing %q", compiled != nil, err, tt.wantErr)
			}
		})
	}
}

func TestCompileReportsTheCELExpressionThatDoesNotParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		source   string
		wantPath string
	}{
		{name: "open condition", source: editedSpec("openWhen: features.mean_value > 1.0", "openWhen: features.mean_value >"), wantPath: "situation.occurrence.openWhen"},
		{name: "close condition", source: editedSpec("closeWhen: features.mean_value <= 1.0", "closeWhen: features.mean_value <="), wantPath: "situation.occurrence.closeWhen"},
		{name: "transition condition", source: editedSpec("      when: features.mean_value > 1.0", "      when: features.mean_value >"), wantPath: "situation.transitions[0].when"},
		{name: "trigger condition", source: editedSpec(`when: situation.phase == "alert"`, `when: situation.phase ==`), wantPath: "cognition.triggers[0].when"},
		{name: "trigger score", source: editedSpec("score: 1.0", `score: "1.0 +"`), wantPath: "cognition.triggers[0].score"},
		{name: "trigger material delta", source: editedSpec("materialDelta: delta.phase_changed", "materialDelta: delta."), wantPath: "cognition.triggers[0].materialDelta"},
		{name: "operator filter", source: editedSpec("    aggregate: mean\n", "    aggregate: mean\n    where: features >\n"), wantPath: "operators[0].where"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileSource(t, tt.source)
			var compileErr *domain.CompileError
			if !errors.As(err, &compileErr) || compileErr.Path != tt.wantPath || !strings.HasPrefix(compileErr.Message, "cel:") {
				t.Fatalf("err = %v, want a cel failure at %s", err, tt.wantPath)
			}
		})
	}
}
