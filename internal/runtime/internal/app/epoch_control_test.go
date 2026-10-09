package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime"
)

func TestADrainedOrKilledEpochKeepsIngestingButStartsNoNewWork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		control func(*runtimecontrol.EpochControl, context.Context, string) error
		refused func(*runtimecontrol.EpochControl, context.Context, string) error
		wantErr error
	}{
		{"drain refuses new admission", (*runtimecontrol.EpochControl).Drain, (*runtimecontrol.EpochControl).AssertAdmission, runtimecontrol.ErrEpochDraining},
		{"kill refuses every later decision", (*runtimecontrol.EpochControl).Kill, (*runtimecontrol.EpochControl).AssertDecision, runtimecontrol.ErrEpochKilled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			epoch := "epoch-" + strings.SplitN(tt.name, " ", 2)[0]
			db := openDatabase(t)
			control := &runtimecontrol.EpochControl{DB: db}
			pipeline := newPipeline(t, db, thingSpec("native", "active"), runtime.PipelineConfig{OwnerEpoch: epoch, EpochControl: control})
			first, err := pipeline.RunJSONL(t.Context(), highLevelTrace(t))
			if err != nil || first.EpisodesAdmitted != 1 || first.CommandsDispatched != 1 {
				t.Fatalf("a live epoch must admit and dispatch: %+v, %v", first, err)
			}
			if err := tt.control(control, t.Context(), epoch); err != nil {
				t.Fatal(err)
			}
			if err := tt.refused(control, t.Context(), epoch); !errors.Is(err, tt.wantErr) {
				t.Fatalf("controlled epoch check = %v, want %v", err, tt.wantErr)
			}
			second, err := pipeline.RunJSONL(t.Context(), writeTrace(t, levelEvent("evt-after-control", "ent-after-control", 15)))
			if err != nil {
				t.Fatalf("a controlled epoch must skip admission, not fail the batch: %v", err)
			}
			if second.EventsIngested != 1 || second.EventsProcessed < 1 || second.EpisodesAdmitted != 0 || second.EpisodesExecuted != 0 || second.CommandsDispatched != 0 {
				t.Fatalf("new work started under a %s epoch: %+v", tt.name, second)
			}
		})
	}
}
