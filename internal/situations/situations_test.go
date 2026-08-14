package situations_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestSituationTransitionsOnFeature(t *testing.T) {
	ctx := context.Background()
	compiled := spec.CompiledSpec{
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		SchemaVersion: "agentic-stream/v1",
		Situation: spec.Situation{
			Type:         "bearing_degradation",
			EntityKey:    "entity.id",
			InitialPhase: "candidate",
			Occurrence: spec.Occurrence{
				OpenWhen:  "features.vibration_rms > 4.5",
				CloseWhen: "features.vibration_rms < 3.0",
			},
			Phases: []spec.Phase{
				{Name: "candidate", Severity: 10},
				{Name: "watch", Severity: 30},
				{Name: "warning", Severity: 60},
			},
			Transitions: []spec.Transition{
				{From: "candidate", To: "watch", When: "features.vibration_rms > 4.5", MinDuration: "0s"},
				{From: "watch", To: "warning", When: "features.vibration_rms > 5.5", MinDuration: "0s"},
			},
			Reducers: []spec.Reducer{
				{Field: "facts.vibration_rms", Strategy: "latest_event_time", Input: "vibration_rms"},
			},
		},
	}

	eng, err := situations.NewEngine("d1", "default", 0, &compiled, ids.Deterministic())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// First feature opens the situation and moves to watch.
	versions, err := eng.ApplyFeature(ctx, operators.Feature{
		OutputName:    "vibration_rms",
		EntityType:    "motor",
		EntityID:      "motor-17",
		Value:         5.0,
		EventTime:     base,
		Watermark:     base,
		InputEventIDs: []string{"evt-1"},
	}, base)
	if err != nil {
		t.Fatalf("ApplyFeature: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected a situation version")
	}
	if versions[0].Phase != "watch" {
		t.Fatalf("expected phase watch, got %s", versions[0].Phase)
	}

	// Second feature moves to warning.
	versions, err = eng.ApplyFeature(ctx, operators.Feature{
		OutputName:    "vibration_rms",
		EntityType:    "motor",
		EntityID:      "motor-17",
		Value:         6.0,
		EventTime:     base.Add(time.Minute),
		Watermark:     base.Add(time.Minute),
		InputEventIDs: []string{"evt-2"},
	}, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("ApplyFeature: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected a situation version")
	}
	if versions[0].Phase != "warning" {
		t.Fatalf("expected phase warning, got %s", versions[0].Phase)
	}
}

func TestSituationPublishesCompletenessChangeFromSourceHealth(t *testing.T) {
	compiled := spec.CompiledSpec{
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		SchemaVersion: "agentic-stream/v1",
		Situation: spec.Situation{
			Type:         "bearing_degradation",
			InitialPhase: "watch",
			Occurrence:   spec.Occurrence{OpenWhen: "true", CloseWhen: "false"},
			Phases:       []spec.Phase{{Name: "watch", Severity: 30}},
			Reducers:     []spec.Reducer{{Field: "facts.heartbeat_missing_5m", Strategy: "latest_event_time", Input: "heartbeat_missing_5m"}},
		},
	}
	eng, err := situations.NewEngine("d1", "default", 0, &compiled, ids.Deterministic())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	versions, err := eng.ApplyFeature(context.Background(), operators.Feature{
		OutputName: "heartbeat_missing_5m", EntityType: "motor", EntityID: "motor-17",
		Value: false, Completeness: string(operators.CompletenessOnTime),
		EventTime: base, Watermark: base, InputEventIDs: []string{"hb-1"},
	}, base)
	if err != nil || len(versions) != 1 || versions[0].Completeness != string(operators.CompletenessOnTime) {
		t.Fatalf("healthy source version=%+v err=%v", versions, err)
	}
	detectionTime := base.Add(5*time.Minute + 30*time.Second)
	versions, err = eng.ApplyFeature(context.Background(), operators.Feature{
		OutputName: "heartbeat_missing_5m", EntityType: "motor", EntityID: "motor-17",
		Value: true, Completeness: string(operators.CompletenessUncertain),
		EventTime: detectionTime, Watermark: detectionTime, InputEventIDs: []string{"hb-1"},
	}, detectionTime)
	if err != nil || len(versions) != 1 || versions[0].Version != 2 || versions[0].Completeness != string(operators.CompletenessUncertain) {
		t.Fatalf("source-health completeness version=%+v err=%v", versions, err)
	}
	if got := versions[0].Facts["facts.heartbeat_missing_5m"]; got != true {
		t.Fatalf("latest_event_time reducer fact = %v, want true", got)
	}
}

func TestSituationSkipsNilFactWhenBuildingFeatures(t *testing.T) {
	compiled := spec.CompiledSpec{
		Digest:        "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		SchemaVersion: "agentic-stream/v1",
		Operators: []spec.Operator{
			{Name: "level_latest", Kind: "aggregate", Output: "level"},
		},
		Situation: spec.Situation{
			Type:         "test",
			InitialPhase: "candidate",
			Occurrence:   spec.Occurrence{OpenWhen: "features.level > 10"},
			Phases:       []spec.Phase{{Name: "candidate", Severity: 10}},
			Reducers:     []spec.Reducer{{Field: "facts.level", Strategy: "latest_event_time", Input: "level"}},
		},
	}
	eng, err := situations.NewEngine("d1", "default", 0, &compiled, ids.Deterministic())
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	versions, err := eng.ApplyFeature(context.Background(), operators.Feature{
		OutputName: "level",
		EntityType: "thing",
		EntityID:   "thing-1",
		Value:      nil,
		EventTime:  base,
		Watermark:  base,
	}, base)
	if err != nil {
		t.Fatalf("ApplyFeature: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected nil feature not to open a situation, got %d versions", len(versions))
	}
}
