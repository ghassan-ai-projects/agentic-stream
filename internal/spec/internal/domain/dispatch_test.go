package domain_test

import (
	"slices"
	"testing"
)

func TestOnlyAnActiveSpecDispatchesItsIntents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, policy string
		want         []string
	}{
		{"active dispatches every declared intent", "    dispatchPolicy: active\n", []string{"notify"}},
		{"shadow dispatches nothing", "    dispatchPolicy: shadow\n", nil},
		{"an undeclared policy is shadow and dispatches nothing", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			compiled, err := compileSource(t, editedSpec("    name: fake\n", "    name: fake\n"+tt.policy))
			if err != nil {
				t.Fatal(err)
			}
			if got := compiled.DispatchedIntentTypes(); !slices.Equal(got, tt.want) {
				t.Fatalf("DispatchedIntentTypes = %v, want %v", got, tt.want)
			}
		})
	}
}
