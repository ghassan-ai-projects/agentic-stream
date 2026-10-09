package domain

import (
	"strings"
	"testing"
)

func TestTargetResolutionPreservesFallbackPrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parameters map[string]any
		want       string
	}{
		{"target", map[string]any{"target": " motor/1 ", "entity_id": "other"}, "motor/1"},
		{"entity", map[string]any{"entity_id": " motor/2 "}, "motor/2"},
		{"nonstring target", map[string]any{"target": 1, "entity_id": "other"}, "other"},
		{"invalid target wins", map[string]any{"target": " ", "entity_id": "other"}, "intent"},
		{"control", map[string]any{"target": "motor\n1"}, "intent"},
		{"length", map[string]any{"target": strings.Repeat("a", 257)}, "intent"},
		{"longest accepted", map[string]any{"target": strings.Repeat("a", 256)}, strings.Repeat("a", 256)},
		{"missing", nil, "intent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizedTarget("intent", test.parameters); got != test.want {
				t.Fatalf("target = %q, want %q", got, test.want)
			}
		})
	}
}
