package operators_test

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestAggregateMean(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", meanSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		env := contractsv1.Envelope{
			ID:             ids.NewSequence("evt_").New(),
			Type:           "sensor.temperature",
			SchemaVersion:  "1.0",
			TenantID:       "default",
			Source:         "test",
			PartitionKey:   "motor-17",
			Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
			EventTime:      base.Add(time.Duration(i) * time.Minute),
			IngestedAt:     base.Add(time.Duration(i) * time.Minute),
			Classification: contractsv1.ClassificationInternal,
			Data:           map[string]any{"celsius": float64(i + 1)},
		}
		fs, nps, err := rt.ApplyEventAt(ctx, ps, env, env.EventTime, env.IngestedAt)
		if err != nil {
			t.Fatalf("ApplyEvent: %v", err)
		}
		ps = nps
		if i == 2 && len(fs) == 0 {
			t.Fatal("expected feature on third event")
		}
		if i == 2 && len(fs) > 0 {
			if fs[0].OutputName != "mean_value" {
				t.Fatalf("unexpected output %q", fs[0].OutputName)
			}
			if got, want := fs[0].Value, 2.0; got != want {
				t.Fatalf("mean = %v, want %v", got, want)
			}
		}
	}
}

func TestSlope(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", slopeSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		env := contractsv1.Envelope{
			ID:             ids.NewSequence("evt_").New(),
			Type:           "sensor.temperature",
			SchemaVersion:  "1.0",
			TenantID:       "default",
			Source:         "test",
			PartitionKey:   "motor-17",
			Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
			EventTime:      base.Add(time.Duration(i) * time.Hour),
			IngestedAt:     base.Add(time.Duration(i) * time.Hour),
			Classification: contractsv1.ClassificationInternal,
			Data:           map[string]any{"celsius": float64(i)},
		}
		fs, nps, err := rt.ApplyEventAt(ctx, ps, env, env.EventTime, env.IngestedAt)
		if err != nil {
			t.Fatalf("ApplyEvent: %v", err)
		}
		ps = nps
		if i == 2 && len(fs) == 0 {
			t.Fatal("expected slope feature")
		}
	}
}

func TestLateEventCorrectsPreviouslyEmittedWindow(t *testing.T) {
	compiled := meanSpec()
	compiled.Time = spec.TimePolicy{
		MaxOutOfOrderness: "0s",
		AllowedLateness:   "5m",
		LatePolicy:        "correct_and_reconsider",
	}
	rt, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := contractsv1.Envelope{
		ID:             "evt-first",
		Type:           "sensor.temperature",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      base,
		IngestedAt:     base,
		Classification: contractsv1.ClassificationInternal,
		Data:           map[string]any{"celsius": 10.0},
	}
	if _, ps, err = rt.ApplyEventAt(ctx, ps, first, base, first.IngestedAt); err != nil {
		t.Fatalf("apply first event: %v", err)
	}

	late := first
	late.ID = "evt-late"
	late.EventTime = base.Add(-time.Minute)
	features, _, err := rt.ApplyEventAt(ctx, ps, late, base, late.IngestedAt)
	if err != nil {
		t.Fatalf("apply late event: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("late event emitted %d features, want 1", len(features))
	}
	if got := features[0].Completeness; got != string(operators.CompletenessCorrected) {
		t.Fatalf("late feature completeness = %q, want corrected", got)
	}
	if got, want := features[0].Value, 10.0; got != want {
		t.Fatalf("late feature value = %v, want %v", got, want)
	}
	if got, want := len(features[0].InputEventIDs), 2; got != want {
		t.Fatalf("late feature input event count = %d, want %d", got, want)
	}

	inOrder := first
	inOrder.ID = "evt-in-order"
	inOrder.EventTime = base.Add(time.Minute)
	features, _, err = rt.ApplyEventAt(ctx, ps, inOrder, base.Add(time.Minute), inOrder.IngestedAt)
	if err != nil {
		t.Fatalf("apply in-order event: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("in-order event emitted %d features, want 1", len(features))
	}
	if got := features[0].Completeness; got == string(operators.CompletenessCorrected) {
		t.Fatal("in-order feature was incorrectly marked corrected")
	}
}

func TestMissingHeartbeatTimerUsesDetectionTime(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	heartbeatEventTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	heartbeatProcessingTime := heartbeatEventTime.Add(30 * time.Second)
	heartbeat := contractsv1.Envelope{
		ID:             "hb-1",
		Type:           "test.heartbeat",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      heartbeatEventTime,
		IngestedAt:     heartbeatProcessingTime,
		Classification: contractsv1.ClassificationInternal,
	}
	features, ps, err := rt.ApplyEventAt(ctx, ps, heartbeat, heartbeatEventTime, heartbeatProcessingTime)
	if err != nil {
		t.Fatalf("apply heartbeat: %v", err)
	}
	if len(features) != 1 || features[0].Value != false {
		t.Fatalf("heartbeat feature = %+v, want one false feature", features)
	}

	detectionTime := heartbeatProcessingTime.Add(5 * time.Minute)
	features, _, err = rt.ApplyTimer(ctx, ps, detectionTime, detectionTime)
	if err != nil {
		t.Fatalf("apply heartbeat timer: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("timer emitted %d features, want 1", len(features))
	}
	feature := features[0]
	if feature.Value != true {
		t.Fatalf("timer feature value = %v, want true", feature.Value)
	}
	if feature.Completeness != string(operators.CompletenessUncertain) {
		t.Fatalf("timer feature completeness = %q, want uncertain", feature.Completeness)
	}
	if !feature.EventTime.Equal(detectionTime) {
		t.Fatalf("timer feature event time = %s, want detection time %s", feature.EventTime, detectionTime)
	}
	if !feature.WindowStart.Equal(heartbeatEventTime) {
		t.Fatalf("timer feature window start = %s, want heartbeat event time %s", feature.WindowStart, heartbeatEventTime)
	}
	if len(feature.InputEventIDs) != 1 || feature.InputEventIDs[0] != heartbeat.ID {
		t.Fatalf("timer feature input IDs = %v, want [%s]", feature.InputEventIDs, heartbeat.ID)
	}
}

func TestMissingHeartbeatEventUsesDetectionTimeWhenLate(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	ctx := context.Background()
	ps := &operators.PartitionState{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	env := contractsv1.Envelope{
		ID:             "hb-late",
		Type:           "test.heartbeat",
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      base,
		IngestedAt:     base.Add(6 * time.Minute),
		Classification: contractsv1.ClassificationInternal,
	}
	detectionTime := base.Add(6 * time.Minute)
	features, _, err := rt.ApplyEventAt(ctx, ps, env, base.Add(5*time.Minute), detectionTime)
	if err != nil {
		t.Fatalf("apply late heartbeat: %v", err)
	}
	if len(features) != 1 || features[0].Value != true {
		t.Fatalf("late heartbeat feature = %+v, want one true feature", features)
	}
	if !features[0].EventTime.Equal(detectionTime) {
		t.Fatalf("late heartbeat feature event time = %s, want detection time %s", features[0].EventTime, detectionTime)
	}
}

func TestOnCloseWindowEmitsAtWatermarkSlideBoundary(t *testing.T) {
	compiled := meanSpec()
	// An omitted emit value is normalized to the schema/runtime default.
	compiled.Windows[0] = spec.Window{Name: "w1", Kind: "sliding", Size: "5m", Slide: "2m"}
	rt, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ps := &operators.PartitionState{}
	for _, event := range []struct {
		id     string
		offset time.Duration
		value  float64
		want   int
	}{
		{id: "sample-0", offset: 0, value: 1, want: 0},
		{id: "sample-1", offset: time.Minute, value: 3, want: 0},
		{id: "sample-2", offset: 2 * time.Minute, value: 5, want: 1},
		{id: "sample-3", offset: 3 * time.Minute, value: 7, want: 0},
	} {
		env := testOperatorEnvelope(event.id, "sensor.temperature", base.Add(event.offset), map[string]any{
			"celsius": event.value,
			"quality": "valid",
		})
		features, next, err := rt.ApplyEventAt(context.Background(), ps, env, env.EventTime, env.IngestedAt)
		if err != nil {
			t.Fatalf("apply %s: %v", event.id, err)
		}
		ps = next
		if got := len(features); got != event.want {
			t.Fatalf("features after %s = %d, want %d", event.id, got, event.want)
		}
		if event.want == 1 {
			feature := features[0]
			if got, want := feature.Value, 3.0; got != want {
				t.Fatalf("closed mean = %v, want %v", got, want)
			}
			if got := feature.Completeness; got != string(operators.CompletenessFinalByPolicy) {
				t.Fatalf("closed completeness = %q, want final_by_policy", got)
			}
			if got, want := feature.WindowEnd, env.EventTime; !got.Equal(want) {
				t.Fatalf("closed window end = %s, want %s", got, want)
			}
		}
	}
}

func TestUnsupportedWindowConfigurationFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		window spec.Window
	}{
		{name: "count window", window: spec.Window{Name: "w1", Kind: "count", Count: 2}},
		{name: "decay window", window: spec.Window{Name: "w1", Kind: "decay", HalfLife: "1m"}},
		{name: "zero sliding size", window: spec.Window{Name: "w1", Kind: "sliding", Size: "0s", Slide: "1s"}},
		{name: "slide exceeds size", window: spec.Window{Name: "w1", Kind: "sliding", Size: "1m", Slide: "2m"}},
		{name: "tumbling slide", window: spec.Window{Name: "w1", Kind: "tumbling", Size: "1m", Slide: "1s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled := meanSpec()
			compiled.Windows = []spec.Window{tt.window}
			if _, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic()); err == nil {
				t.Fatal("NewOperatorRuntime succeeded for unsupported window configuration")
			}
		})
	}
}

func TestEarlyAndCloseWindowEmitsProvisionalUpdates(t *testing.T) {
	compiled := meanSpec()
	compiled.Windows[0] = spec.Window{Name: "w1", Kind: "sliding", Size: "5m", Slide: "2m", Emit: "early_and_close"}
	rt, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ps := &operators.PartitionState{}
	first := testOperatorEnvelope("sample-0", "sensor.temperature", base, map[string]any{"celsius": 1.0, "quality": "valid"})
	features, ps, err := rt.ApplyEventAt(context.Background(), ps, first, base, base)
	if err != nil {
		t.Fatalf("apply first event: %v", err)
	}
	if len(features) != 1 || features[0].Completeness != string(operators.CompletenessProvisional) {
		t.Fatalf("first features = %+v, want one provisional feature", features)
	}

	second := testOperatorEnvelope("sample-2", "sensor.temperature", base.Add(2*time.Minute), map[string]any{"celsius": 5.0, "quality": "valid"})
	features, _, err = rt.ApplyEventAt(context.Background(), ps, second, second.EventTime, second.IngestedAt)
	if err != nil {
		t.Fatalf("apply closing event: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("closing features = %d, want one provisional update", len(features))
	}
	if features[0].Completeness != string(operators.CompletenessProvisional) {
		t.Fatalf("closing completeness = %s, want provisional", features[0].Completeness)
	}
}

func TestQualityAdmissionIsPerOperator(t *testing.T) {
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs:        []spec.Input{{Name: "zone", EventType: "zone.temp.observed", SchemaVersion: "1.0", SchemaRef: "zone.temp.observed/1.0", PartitionKey: "entity.id", EntityType: "zone"}},
		Windows:       []spec.Window{{Name: "w1", Kind: "tumbling", Size: "5m", Emit: "on_update"}},
		Operators: []spec.Operator{
			{Name: "temperature", Kind: "aggregate", Inputs: []string{"zone"}, Field: "data.celsius", Window: "w1", Aggregate: "mean", Output: "temperature_mean"},
			{Name: "heartbeat", Kind: "missing_heartbeat", Inputs: []string{"zone"}, Duration: "5m", Output: "heartbeat_missing"},
		},
	}
	rt, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	env := testOperatorEnvelope("zone-event", "zone.temp.observed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]any{
		"celsius": 25.0,
		"quality": "valid",
	})
	features, _, err := rt.ApplyEventAt(context.Background(), &operators.PartitionState{}, env, env.EventTime, env.IngestedAt)
	if err != nil {
		t.Fatalf("apply event: %v", err)
	}
	if len(features) != 1 || features[0].OperatorID != "heartbeat" {
		t.Fatalf("features = %+v, want only heartbeat feature", features)
	}
}

func TestHeartbeatTimerCarriesExplicitTenantAndPartition(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	env := testOperatorEnvelope("heartbeat-1", "test.heartbeat", base, map[string]any{"boot_id": "boot-A"})
	env.TenantID = "tenant-b"
	env.PartitionKey = "partition-b"
	ps := &operators.PartitionState{}
	if _, ps, err = rt.ApplyEventAt(context.Background(), ps, env, base, base); err != nil {
		t.Fatalf("apply heartbeat: %v", err)
	}
	wantPartition := env.PartitionID(0)
	features, _, err := rt.ApplyTimer(context.Background(), ps, base.Add(5*time.Minute), base.Add(5*time.Minute), operators.TimerIdentity{
		TenantID:    env.TenantID,
		PartitionID: wantPartition,
	})
	if err != nil {
		t.Fatalf("apply timer: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("timer feature count = %d, want 1", len(features))
	}
	if got := features[0].TenantID; got != env.TenantID {
		t.Fatalf("timer tenant = %q, want %q", got, env.TenantID)
	}
	if got := features[0].PartitionID; got != wantPartition {
		t.Fatalf("timer partition = %d, want %d", got, wantPartition)
	}
}

func TestApplyTimerHonorsCancellation(t *testing.T) {
	rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = rt.ApplyTimer(ctx, &operators.PartitionState{}, time.Time{}, time.Time{})
	if err != context.Canceled {
		t.Fatalf("ApplyTimer error = %v, want %v", err, context.Canceled)
	}
}

func TestNumericQualityGate(t *testing.T) {
	tests := []struct {
		name         string
		data         map[string]any
		quality      []contractsv1.QualityFlag
		wantFeatures int
	}{
		{name: "valid payload quality", data: map[string]any{"celsius": 42.0, "quality": "valid"}, wantFeatures: 1},
		{name: "invalid payload quality", data: map[string]any{"celsius": 42.0, "quality": "invalid"}},
		{name: "warming payload quality", data: map[string]any{"celsius": 42.0, "quality": "warming"}},
		{name: "disconnected payload quality", data: map[string]any{"celsius": 42.0, "quality": "disconnected"}},
		{name: "rail fault payload quality", data: map[string]any{"celsius": 42.0, "quality": "rail_high"}},
		{name: "invalid envelope quality", data: map[string]any{"celsius": 42.0}, quality: []contractsv1.QualityFlag{{Code: "disconnected"}}},
		{name: "non-finite numeric value", data: map[string]any{"celsius": math.NaN(), "quality": "valid"}},
		{name: "legacy payload without quality", data: map[string]any{"celsius": 42.0}, wantFeatures: 1},
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := operators.NewOperatorRuntime("d1", meanSpec(), ids.Deterministic())
			if err != nil {
				t.Fatalf("NewOperatorRuntime: %v", err)
			}
			env := testOperatorEnvelope("quality", "sensor.temperature", base, tt.data)
			env.Quality = tt.quality
			features, _, err := rt.ApplyEventAt(context.Background(), &operators.PartitionState{}, env, base, env.IngestedAt)
			if err != nil {
				t.Fatalf("ApplyEvent: %v", err)
			}
			if got := len(features); got != tt.wantFeatures {
				t.Fatalf("feature count = %d, want %d", got, tt.wantFeatures)
			}
		})
	}
}

func TestLatestAggregateUsesDeterministicEventTimeOrdering(t *testing.T) {
	tests := []struct {
		name        string
		events      []operatorTestEvent
		wantLatest  float64
		wantMaximum float64
	}{
		{
			name: "event time beats arrival order",
			events: []operatorTestEvent{
				{ID: "late-arrival", Offset: 2 * time.Minute, Value: 10},
				{ID: "older-event-time", Offset: time.Minute, Value: 100},
			},
			wantLatest:  10,
			wantMaximum: 100,
		},
		{
			name: "event id breaks event time tie",
			events: []operatorTestEvent{
				{ID: "event-b", Offset: time.Minute, Value: 2},
				{ID: "event-a", Offset: time.Minute, Value: 3},
			},
			wantLatest:  2,
			wantMaximum: 3,
		},
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled := meanSpec()
			compiled.Operators = []spec.Operator{
				{Name: "latest", Kind: "aggregate", Inputs: []string{"temp"}, Field: "data.celsius", Aggregate: "latest", Window: "w1", Output: "latest_value"},
				{Name: "maximum", Kind: "aggregate", Inputs: []string{"temp"}, Field: "data.celsius", Aggregate: "max", Window: "w1", Output: "max_value"},
			}
			rt, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic())
			if err != nil {
				t.Fatalf("NewOperatorRuntime: %v", err)
			}

			var features []operators.Feature
			ps := &operators.PartitionState{}
			for _, event := range tt.events {
				env := testOperatorEnvelope(event.ID, "sensor.temperature", base.Add(event.Offset), map[string]any{
					"celsius": event.Value,
					"quality": "valid",
				})
				var emitted []operators.Feature
				emitted, ps, err = rt.ApplyEventAt(context.Background(), ps, env, env.EventTime, env.IngestedAt)
				if err != nil {
					t.Fatalf("ApplyEvent: %v", err)
				}
				features = emitted
			}

			values := make(map[string]float64, len(features))
			for _, feature := range features {
				values[feature.OutputName] = feature.Value.(float64)
			}
			if got := values["latest_value"]; got != tt.wantLatest {
				t.Fatalf("latest = %v, want %v", got, tt.wantLatest)
			}
			if got := values["max_value"]; got != tt.wantMaximum {
				t.Fatalf("max = %v, want %v", got, tt.wantMaximum)
			}
		})
	}
}

func TestWindowStateBootBoundaryAndSequenceWrap(t *testing.T) {
	tests := []struct {
		name         string
		events       []operatorTestEvent
		wantValue    float64
		wantInputIDs []string
		wantBootID   string
	}{
		{
			name: "sequence wrap within one boot stays in the window",
			events: []operatorTestEvent{
				{ID: "boot-a-high", Offset: 0, Value: 100, BootID: "boot-A", Sequence: 4294967295},
				{ID: "boot-a-wrap", Offset: time.Minute, Value: 200, BootID: "boot-A", Sequence: 0},
			},
			wantValue:    200,
			wantInputIDs: []string{"boot-a-high", "boot-a-wrap"},
			wantBootID:   "boot-A",
		},
		{
			name: "new boot starts a fresh window",
			events: []operatorTestEvent{
				{ID: "boot-a", Offset: 0, Value: 100, BootID: "boot-A", Sequence: 7},
				{ID: "boot-b", Offset: time.Minute, Value: 3, BootID: "boot-B", Sequence: 0},
			},
			wantValue:    3,
			wantInputIDs: []string{"boot-b"},
			wantBootID:   "boot-B",
		},
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled := meanSpec()
			compiled.Operators[0].Aggregate = "max"
			rt, err := operators.NewOperatorRuntime("d1", compiled, ids.Deterministic())
			if err != nil {
				t.Fatalf("NewOperatorRuntime: %v", err)
			}

			ps := &operators.PartitionState{}
			var features []operators.Feature
			for _, event := range tt.events {
				env := testOperatorEnvelope(event.ID, "sensor.temperature", base.Add(event.Offset), map[string]any{
					"celsius": event.Value,
					"quality": "valid",
					"boot_id": event.BootID,
					"seq":     float64(event.Sequence),
				})
				features, ps, err = rt.ApplyEventAt(context.Background(), ps, env, env.EventTime, env.IngestedAt)
				if err != nil {
					t.Fatalf("ApplyEvent: %v", err)
				}
			}
			if len(features) != 1 {
				t.Fatalf("feature count = %d, want 1", len(features))
			}
			if got := features[0].Value; got != tt.wantValue {
				t.Fatalf("aggregate = %v, want %v", got, tt.wantValue)
			}
			if got := features[0].InputEventIDs; !slices.Equal(got, tt.wantInputIDs) {
				t.Fatalf("input event IDs = %v, want %v", got, tt.wantInputIDs)
			}
			var blob *operators.OperatorStateBlob
			for _, candidate := range ps.OperatorStates["op1"] {
				if candidate != nil && candidate.Window != nil && candidate.Window.BootID == tt.wantBootID {
					blob = candidate
					break
				}
			}
			if blob == nil || blob.Window == nil {
				t.Fatal("window state was not persisted")
			}
			if blob.Window.BootID != tt.wantBootID {
				t.Fatalf("boot ID = %q, want %q", blob.Window.BootID, tt.wantBootID)
			}
		})
	}
}

func TestStaleBootCannotMutateWindow(t *testing.T) {
	t.Parallel()
	rt, err := operators.NewOperatorRuntime("d1", meanSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ps := &operators.PartitionState{}
	apply := func(id, boot string, value float64, at time.Time) {
		t.Helper()
		env := testOperatorEnvelope(id, "sensor.temperature", at, map[string]any{
			"celsius": value,
			"quality": "valid",
			"boot_id": boot,
		})
		var err error
		_, ps, err = rt.ApplyEventAt(context.Background(), ps, env, at, env.IngestedAt)
		if err != nil {
			t.Fatalf("apply %s: %v", id, err)
		}
	}
	apply("boot-a", "boot-A", 10, base)
	apply("boot-b", "boot-B", 20, base.Add(time.Minute))
	apply("stale-a", "boot-A", 999, base.Add(2*time.Minute))

	var current *operators.OperatorStateBlob
	for _, candidate := range ps.OperatorStates["op1"] {
		if candidate != nil && candidate.Window != nil && candidate.Window.BootID == "boot-B" {
			current = candidate
		}
	}
	if current == nil || len(current.Window.Samples) != 1 || current.Window.Samples[0].EventID != "boot-b" {
		t.Fatalf("stale boot mutated active window: %+v", ps.OperatorStates["op1"])
	}
}

func TestHeartbeatTimerSkipsPreviousBootState(t *testing.T) {
	t.Parallel()
	rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ps := &operators.PartitionState{}
	for _, event := range []struct {
		id, boot string
		at       time.Time
	}{
		{"hb-a", "boot-A", base},
		{"hb-b", "boot-B", base.Add(time.Minute)},
	} {
		env := testOperatorEnvelope(event.id, "test.heartbeat", event.at, map[string]any{"boot_id": event.boot})
		var applyErr error
		_, ps, applyErr = rt.ApplyEventAt(context.Background(), ps, env, event.at, event.at)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.id, applyErr)
		}
	}
	features, _, err := rt.ApplyTimer(context.Background(), ps, base.Add(6*time.Minute), base.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("apply timer: %v", err)
	}
	if len(features) != 1 || features[0].BootID != "boot-B" {
		t.Fatalf("timer features = %+v, want only active boot-B feature", features)
	}
}

func TestHeartbeatTimerSkipsBootlessStateAfterBootAdmission(t *testing.T) {
	t.Parallel()
	rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ps := &operators.PartitionState{}
	for _, event := range []struct {
		id, boot string
		at       time.Time
	}{
		{"hb-legacy", "", base},
		{"hb-identified", "boot-A", base.Add(time.Minute)},
	} {
		data := map[string]any{}
		if event.boot != "" {
			data["boot_id"] = event.boot
		}
		env := testOperatorEnvelope(event.id, "test.heartbeat", event.at, data)
		var applyErr error
		_, ps, applyErr = rt.ApplyEventAt(context.Background(), ps, env, event.at, event.at)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.id, applyErr)
		}
	}
	features, _, err := rt.ApplyTimer(context.Background(), ps, base.Add(6*time.Minute), base.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("apply timer: %v", err)
	}
	if len(features) != 1 || features[0].BootID != "boot-A" {
		t.Fatalf("timer features = %+v, want only identified boot feature", features)
	}
}

func TestHeartbeatTimerFeatureCarriesBootScopedStateKey(t *testing.T) {
	t.Parallel()
	rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
	if err != nil {
		t.Fatalf("NewOperatorRuntime: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	env := testOperatorEnvelope("hb-boot", "test.heartbeat", base, map[string]any{"boot_id": "boot-A"})
	ps := &operators.PartitionState{}
	if _, ps, err = rt.ApplyEventAt(context.Background(), ps, env, base, base); err != nil {
		t.Fatalf("apply heartbeat: %v", err)
	}
	features, _, err := rt.ApplyTimer(context.Background(), ps, base.Add(5*time.Minute), base.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("apply timer: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("timer feature count = %d, want 1", len(features))
	}
	want := "motor-17\x1fboot-A"
	if features[0].StateKey != want || features[0].BootID != "boot-A" {
		t.Fatalf("timer identity = state key %q boot %q, want %q/boot-A", features[0].StateKey, features[0].BootID, want)
	}
}

func TestInvalidHeartbeatDoesNotRefreshLiveness(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]any
		quality []contractsv1.QualityFlag
	}{
		{name: "invalid payload quality", data: map[string]any{"quality": "invalid", "boot_id": "boot-B"}},
		{name: "warming payload quality", data: map[string]any{"quality": "warming", "boot_id": "boot-B"}},
		{name: "disconnected envelope quality", data: map[string]any{"boot_id": "boot-B"}, quality: []contractsv1.QualityFlag{{Code: "disconnected"}}},
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := operators.NewOperatorRuntime("d1", heartbeatSpec(), ids.Deterministic())
			if err != nil {
				t.Fatalf("NewOperatorRuntime: %v", err)
			}
			ps := &operators.PartitionState{}
			valid := testOperatorEnvelope("heartbeat-valid", "test.heartbeat", base, map[string]any{"quality": "valid", "boot_id": "boot-A"})
			features, ps, err := rt.ApplyEventAt(context.Background(), ps, valid, base, base)
			if err != nil {
				t.Fatalf("apply valid heartbeat: %v", err)
			}
			if len(features) != 1 || features[0].Value != false {
				t.Fatalf("valid heartbeat feature = %+v, want one false feature", features)
			}

			invalid := testOperatorEnvelope("heartbeat-invalid", "test.heartbeat", base.Add(4*time.Minute), tt.data)
			invalid.Quality = tt.quality
			features, ps, err = rt.ApplyEventAt(context.Background(), ps, invalid, base.Add(4*time.Minute), base.Add(4*time.Minute))
			if err != nil {
				t.Fatalf("apply invalid heartbeat: %v", err)
			}
			if len(features) != 0 {
				t.Fatalf("invalid heartbeat emitted %d features, want 0", len(features))
			}

			features, _, err = rt.ApplyTimer(context.Background(), ps, base.Add(5*time.Minute), base.Add(5*time.Minute))
			if err != nil {
				t.Fatalf("apply heartbeat timer: %v", err)
			}
			if len(features) != 1 || features[0].Value != true {
				t.Fatalf("timer features = %+v, want one missing feature", features)
			}
		})
	}
}

type operatorTestEvent struct {
	ID       string
	Offset   time.Duration
	Value    float64
	BootID   string
	Sequence uint64
}

func testOperatorEnvelope(id, eventType string, eventTime time.Time, data map[string]any) contractsv1.Envelope {
	return contractsv1.Envelope{
		ID:             id,
		Type:           eventType,
		SchemaVersion:  "1.0",
		TenantID:       "default",
		Source:         "test",
		PartitionKey:   "motor-17",
		Entity:         contractsv1.EntityRef{Type: "motor", ID: "motor-17"},
		EventTime:      eventTime,
		IngestedAt:     eventTime,
		Classification: contractsv1.ClassificationInternal,
		Data:           data,
	}
}

func meanSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs: []spec.Input{
			{Name: "temp", EventType: "sensor.temperature", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		},
		Windows: []spec.Window{
			{Name: "w1", Kind: "sliding", Size: "5m", Slide: "1m", Emit: "on_update"},
		},
		Operators: []spec.Operator{
			{Name: "op1", Kind: "aggregate", Inputs: []string{"temp"}, Field: "data.celsius", Window: "w1", Aggregate: "mean", Output: "mean_value", Unit: "celsius"},
		},
	}
}

func slopeSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs: []spec.Input{
			{Name: "temp", EventType: "sensor.temperature", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		},
		Windows: []spec.Window{
			{Name: "w1", Kind: "sliding", Size: "6h", Slide: "15m", Emit: "on_update"},
		},
		Operators: []spec.Operator{
			{Name: "op1", Kind: "slope", Inputs: []string{"temp"}, Field: "data.celsius", Window: "w1", Aggregate: "slope", Output: "slope_value", Unit: "celsius_per_hour"},
		},
	}
}

func heartbeatSpec() *spec.CompiledSpec {
	return &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Inputs: []spec.Input{
			{Name: "heartbeat", EventType: "test.heartbeat", SchemaVersion: "1.0", PartitionKey: "entity.id", EntityType: "motor"},
		},
		Operators: []spec.Operator{
			{Name: "heartbeat_missing", Kind: "missing_heartbeat", Inputs: []string{"heartbeat"}, Duration: "5m", Output: "heartbeat_missing_5m"},
		},
	}
}
