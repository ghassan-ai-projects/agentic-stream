package domain

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func schemaEnum(t *testing.T, property string) []string {
	t.Helper()
	raw, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	for _, definition := range schema.Defs {
		if values := definition.Properties[property].Enum; len(values) > 0 {
			return values
		}
	}
	t.Fatalf("schema declares no enum for %s", property)
	return nil
}

func TestSchemaEnumsMatchDeclaredVocabulary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		property string
		want     []string
	}{
		{property: "dispatchPolicy", want: []string{DispatchActive, DispatchShadow}},
		{property: "lane", want: []string{LaneFast, LaneDeep}},
	}
	for _, tt := range tests {
		t.Run(tt.property, func(t *testing.T) {
			t.Parallel()
			got := slices.Clone(schemaEnum(t, tt.property))
			slices.Sort(got)
			slices.Sort(tt.want)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("schema %s enum = %v, vocabulary = %v", tt.property, got, tt.want)
			}
		})
	}
}

func TestUnsetDispatchPolicyIsShadow(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, declared, want string }{
		{name: "unset", declared: "", want: DispatchShadow},
		{name: "shadow", declared: DispatchShadow, want: DispatchShadow},
		{name: "active", declared: DispatchActive, want: DispatchActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := EffectiveDispatchPolicy(tt.declared); got != tt.want {
				t.Fatalf("EffectiveDispatchPolicy(%q) = %q, want %q", tt.declared, got, tt.want)
			}
			cognition := Cognition{Executor: Executor{DispatchPolicy: tt.declared}}
			defaultCognition(&cognition)
			if cognition.Executor.DispatchPolicy != tt.want {
				t.Fatalf("compiled default = %q, want %q", cognition.Executor.DispatchPolicy, tt.want)
			}
		})
	}
}
