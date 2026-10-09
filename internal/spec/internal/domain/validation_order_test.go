package domain_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCompilePreservesValidationPrecedence(t *testing.T) {
	t.Parallel()
	base := minimalSpecYAML()
	tests := []struct {
		name, source, want string
	}{
		{
			name:   "duplicate before unknown fields and identity",
			source: strings.Replace(base, "apiVersion: agentic-stream/v1", "apiVersion: wrong\nunknownField: true", 1) + "kind: Other\n",
			want:   "duplicate key",
		},
		{
			name:   "unknown field before identity",
			source: strings.Replace(base, "apiVersion: agentic-stream/v1", "apiVersion: wrong\nunknownField: true", 1),
			want:   "field unknownField not found",
		},
		{
			name:   "api version before kind",
			source: strings.Replace(strings.Replace(base, "apiVersion: agentic-stream/v1", "apiVersion: wrong", 1), "kind: SituationSpec", "kind: Other", 1),
			want:   "expected agentic-stream/v1",
		},
		{
			name:   "kind before schema",
			source: strings.Replace(strings.Replace(base, "kind: SituationSpec", "kind: Other", 1), "aggregate: mean", "aggregate: unsupported", 1),
			want:   "expected SituationSpec",
		},
		{
			name:   "schema before references",
			source: strings.Replace(strings.Replace(base, "aggregate: mean", "aggregate: unsupported", 1), "input: mean_value", "input: unknown", 1),
			want:   "schema validation",
		},
		{
			name:   "references before expressions",
			source: strings.Replace(strings.Replace(base, "input: mean_value", "input: unknown", 1), "openWhen: features.mean_value > 1.0", "openWhen: invalid(", 1),
			want:   "unknown operator output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			compiled, err := compileSource(t, tt.source)
			if err == nil || !strings.Contains(err.Error(), tt.want) || compiled != nil {
				t.Fatalf("compile: hasCompiled=%t err=%v, want %q", compiled != nil, err, tt.want)
			}
		})
	}
}

func TestCompileGivesJSONAndYAMLSourcesTheSameDigest(t *testing.T) {
	t.Parallel()
	source := []byte(strings.Replace(minimalSpecYAML(), "score: 1.0", `score: "1.0"`, 1))
	first, err := compileSource(t, string(source))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(source, &document); err != nil {
		t.Fatal(err)
	}
	jsonSource, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compileSource(t, string(jsonSource))
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || !bytes.Equal(first.CanonicalJSON, second.CanonicalJSON) {
		t.Fatalf("equivalent documents changed identity: %s != %s", first.Digest, second.Digest)
	}
}
