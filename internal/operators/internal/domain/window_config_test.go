package domain_test

import (
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func requireErrorContaining(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}

func TestRuntimeRefusesAWindowItCannotRunExactly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		window  spec.Window
		wantErr string
	}{
		{"count window", spec.Window{Kind: "count", Count: 2}, `unsupported window kind "count"`},
		{"decay window", spec.Window{Kind: "decay", HalfLife: "1m"}, `unsupported window kind "decay"`},
		{"unknown emit mode", spec.Window{Kind: "tumbling", Size: "1m", Emit: "whenever"}, `unsupported emit mode "whenever"`},
		{"tumbling with slide", spec.Window{Kind: "tumbling", Size: "1m", Slide: "1s"}, "tumbling windows do not support slide"},
		{"tumbling without size", spec.Window{Kind: "tumbling"}, "tumbling size:"},
		{"tumbling with negative size", spec.Window{Kind: "tumbling", Size: "-1m"}, "tumbling size must be positive"},
		{"sliding with unreadable size", spec.Window{Kind: "sliding", Size: "soon", Slide: "1s"}, "sliding size:"},
		{"sliding with unreadable slide", spec.Window{Kind: "sliding", Size: "1m", Slide: "soon"}, "sliding slide:"},
		{"sliding with zero size", spec.Window{Kind: "sliding", Size: "0s", Slide: "1s"}, "sliding size must be positive"},
		{"sliding with zero slide", spec.Window{Kind: "sliding", Size: "1m", Slide: "0s"}, "sliding slide must be positive"},
		{"slide exceeds size", spec.Window{Kind: "sliding", Size: "1m", Slide: "2m"}, "sliding slide 2m0s exceeds size 1m0s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			compiled := meanSpec()
			tc.window.Name = "w1"
			compiled.Windows = []spec.Window{tc.window}
			_, err := domain.NewOperatorRuntime("d1", compiled, sources.Deterministic())
			requireErrorContaining(t, err, "window w1: "+tc.wantErr)
		})
	}
}

func TestRuntimeAcceptsTumblingAndSlidingWindows(t *testing.T) {
	t.Parallel()
	for _, window := range []spec.Window{
		{Kind: "tumbling", Size: "1m"},
		{Kind: "sliding", Size: "1m", Slide: "1m", Emit: "early_and_close"},
	} {
		compiled := meanSpec()
		window.Name = "w1"
		compiled.Windows = []spec.Window{window}
		if _, err := domain.NewOperatorRuntime("d1", compiled, sources.Deterministic()); err != nil {
			t.Errorf("window %+v refused: %v", window, err)
		}
	}
}

func TestRuntimeRefusesAnOperatorWhoseWindowIsUndeclared(t *testing.T) {
	t.Parallel()
	compiled := meanSpec()
	compiled.Operators[0].Window = "missing"
	_, err := domain.NewOperatorRuntime("d1", compiled, sources.Deterministic())
	requireErrorContaining(t, err, "operator op1 references unknown window missing")
}

func TestUnsupportedOperatorKindAndAggregateFailClosedWhenEventsArrive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		operator spec.Operator
		wantErr  string
	}{
		{"operator kind", spec.Operator{Kind: "percentile", Aggregate: "mean", Output: "x"}, `unsupported operator kind "percentile"`},
		{"aggregate", spec.Operator{Kind: "aggregate", Aggregate: "median", Output: "x"}, `unsupported aggregate "median"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, temperatureSpec(slidingWindow("5m", "1m", "on_update"), tc.operator))
			env := temperature("t", base, 1)
			_, _, err := h.rt.ApplyEventAt(t.Context(), h.ps, env, base, base)
			requireErrorContaining(t, err, tc.wantErr)
		})
	}
}
