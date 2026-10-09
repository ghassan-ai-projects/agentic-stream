package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestEngineCreatesTriggerAndSchedulerItem(t *testing.T) {
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	compiled := spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Time: spec.TimePolicy{
			MaxOutOfOrderness: "2m",
		},
		Inputs: []spec.Input{
			{Name: "level", EventType: "test.observed", EntityType: "thing"},
		},
		Windows: []spec.Window{
			{Name: "tiny", Kind: "tumbling", Size: "1m", Emit: "early_and_close"},
		},
		Operators: []spec.Operator{
			{Name: "level_latest", Kind: "aggregate", Inputs: []string{"level"}, Field: "data.level", Aggregate: "max", Window: "tiny", Output: "level"},
		},
		Situation: spec.Situation{
			Type:         "test",
			InitialPhase: "candidate",
			Phases:       []spec.Phase{{Name: "candidate", Severity: 10}},
			Occurrence: spec.Occurrence{
				OpenWhen: "features.level > 10",
			},
			Reducers: []spec.Reducer{
				{Field: "facts.level", Strategy: "latest_event_time", Input: "level"},
			},
		},
		Cognition: spec.Cognition{
			Triggers: []spec.Trigger{
				{
					Name:          "high",
					When:          "features.level > 10",
					Score:         "situation.severity",
					Threshold:     5,
					Lane:          "fast",
					MaterialDelta: "delta.phase_changed",
				},
			},
		},
	}

	log := eventlog.NewEventLog(db)
	eng, err := newService(ctx, db, log, sources.Physical(), &compiled, "default", true)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	appendObservedEvent(t, log, "test.observed", "sim", contractsv1.EntityRef{Type: "thing", ID: "ent-1"}, map[string]any{"level": 15.0})

	if _, err := eng.RunGlobal(ctx, nil); err != nil {
		t.Fatalf("run engine: %v", err)
	}

	var outcome string
	if err := db.QueryRowContext(ctx,
		"SELECT outcome FROM trigger_evaluations WHERE tenant_id = ?", "default",
	).Scan(&outcome); err != nil {
		t.Fatalf("query trigger: %v", err)
	}
	if outcome != "admitted" {
		t.Fatalf("expected admitted trigger, got %s", outcome)
	}

	var itemCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scheduler_items WHERE tenant_id = ?", "default",
	).Scan(&itemCount); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if itemCount != 1 {
		t.Fatalf("expected one scheduler item, got %d", itemCount)
	}
}
