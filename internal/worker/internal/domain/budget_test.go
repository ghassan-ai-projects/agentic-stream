package domain

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestBudgetRequiresAValidPositiveWallTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		budget *runtimev1.EpisodeBudget
		want   string
	}{
		{"no budget", nil, "requires a valid wall_time"},
		{"no wall time", &runtimev1.EpisodeBudget{MaxModelCalls: 5}, "requires a valid wall_time"},
		{"malformed wall time", &runtimev1.EpisodeBudget{WallTime: &durationpb.Duration{Seconds: 1, Nanos: -1}}, "requires a valid wall_time"},
		{"zero wall time", &runtimev1.EpisodeBudget{WallTime: durationpb.New(0)}, "must be positive"},
		{"negative wall time", &runtimev1.EpisodeBudget{WallTime: durationpb.New(-time.Second)}, "must be positive"},
		{"one nanosecond", &runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Nanosecond)}, ""},
		{"one second", &runtimev1.EpisodeBudget{WallTime: durationpb.New(time.Second)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateBudget(tt.budget)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("ValidateBudget() = %v, want %q", err, tt.want)
			}
		})
	}
}
