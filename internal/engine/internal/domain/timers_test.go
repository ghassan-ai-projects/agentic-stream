package domain

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestParseTimerPayloadReadsTheExpectedEvent(t *testing.T) {
	t.Parallel()
	if got, err := ParseTimerPayload("tmr-1", []byte(`{"expected_event_id":"evt-9","extra":1}`)); err != nil || got != "evt-9" {
		t.Fatalf("expected event = %q err=%v", got, err)
	}
	if _, err := ParseTimerPayload("tmr-1", []byte("{")); err == nil || !strings.Contains(err.Error(), "decode timer payload tmr-1") {
		t.Fatalf("err = %v", err)
	}
}

func TestTimerMatchingFencesInactiveBootsAndRequiresEveryTimer(t *testing.T) {
	t.Parallel()
	timers := []DueTimer{
		{ID: "t-live", OperatorID: "hb", StateKey: "m1", ExpectedEventID: "evt-2"},
		{ID: "t-stale", OperatorID: "hb", StateKey: "m2", ExpectedEventID: "evt-7"},
		{ID: "t-old-boot", OperatorID: "hb", StateKey: "m3", ExpectedEventID: "evt-3"},
	}
	matched := InactiveTimerIDs(timers, func(stateKey string) bool { return stateKey != "m3" })
	if _, fenced := matched["t-old-boot"]; len(matched) != 1 || !fenced {
		t.Fatalf("only the inactive boot may be pre-matched: %v", matched)
	}
	features := []operators.Feature{
		{OperatorID: "hb", StateKey: "m1", InputEventIDs: []string{"evt-1", "evt-2"}},
		{OperatorID: "hb", StateKey: "m2", InputEventIDs: []string{"evt-1", "evt-8"}},
		{OperatorID: "hb", EntityID: "m9", InputEventIDs: nil},
	}
	firings := MatchTimerFeatures(timers, features, matched)
	if len(firings) != 1 || firings[0].Timer.ID != "t-live" {
		t.Fatalf("firings = %+v", firings)
	}
	if err := RequireAllTimersMatched(matched, timers); err == nil || !strings.Contains(err.Error(), "matched 2 of 3") {
		t.Fatalf("an unmatched stale timer must refuse the batch: %v", err)
	}
	matched["t-stale"] = struct{}{}
	if err := RequireAllTimersMatched(matched, timers); err != nil {
		t.Fatal(err)
	}
}

func TestTimerFeatureKeysFallBackToTheEntity(t *testing.T) {
	t.Parallel()
	byEntity := []DueTimer{{ID: "t", OperatorID: "hb", StateKey: "m1", ExpectedEventID: "e"}}
	feature := operators.Feature{OperatorID: "hb", EntityID: "m1", InputEventIDs: []string{"e"}}
	if len(MatchTimerFeatures(byEntity, []operators.Feature{feature}, map[string]struct{}{})) != 1 {
		t.Fatal("a feature without a state key must match by entity")
	}
}

func TestDistinctEntitiesKeepFirstSeenOrder(t *testing.T) {
	t.Parallel()
	features := []operators.Feature{{EntityType: "motor", EntityID: "m2"}, {EntityType: "motor", EntityID: "m1"}, {EntityType: "motor", EntityID: "m2"}, {EntityType: "pump", EntityID: "m1"}}
	want := []EntityRef{{Type: "motor", ID: "m2"}, {Type: "motor", ID: "m1"}, {Type: "pump", ID: "m1"}}
	if got := DistinctEntities(features); !slices.Equal(got, want) {
		t.Fatalf("entities = %v, want %v", got, want)
	}
}

func TestTimerWithoutTheExpectedLastEventIsNotMatched(t *testing.T) {
	t.Parallel()
	timers := []DueTimer{{ID: "t", OperatorID: "hb", StateKey: "m1", ExpectedEventID: "evt-2"}}
	features := []operators.Feature{
		{OperatorID: "hb", StateKey: "m1", InputEventIDs: nil},
		{OperatorID: "hb", StateKey: "m1", InputEventIDs: []string{"evt-2", "evt-3"}},
		{OperatorID: "other", StateKey: "m1", InputEventIDs: []string{"evt-2"}},
	}
	if firings := MatchTimerFeatures(timers, features, map[string]struct{}{}); len(firings) != 0 {
		t.Fatalf("firings = %+v, want none: only the last input event of the right operator fires the timer", firings)
	}
}

func TestEnrichTimerFeatureRecordsProvenance(t *testing.T) {
	t.Parallel()
	feature := operators.Feature{Traceparent: "tp", Tracestate: "ts"}
	EnrichTimerFeature(&feature, "tenant", 4, DueTimer{ID: "tmr-1", DueAt: "due"}, base, "virtual")
	if feature.TenantID != "tenant" || feature.PartitionID != 4 || feature.Metadata["timer_id"] != "tmr-1" || feature.Metadata["clock_quality"] != "virtual" ||
		feature.Metadata["source_traceparent"] != "tp" || feature.Metadata["timer_fired_at"] != kernel.FormatTime(base) {
		t.Fatalf("metadata = %v", feature.Metadata)
	}
}

func TestHeartbeatTimersArmOnlyHeartbeatOperatorsWithAnEvent(t *testing.T) {
	t.Parallel()
	last, processed := base, base.Add(time.Minute)
	state := &operators.PartitionState{OperatorStates: map[string]map[string]*operators.OperatorStateBlob{
		"hb": {
			"m1": {Heartbeat: &operators.HeartbeatState{LastEventTime: &last, LastProcessingTime: &processed, LastEventID: "evt-1"}},
			"m2": {Heartbeat: &operators.HeartbeatState{LastEventTime: &last, LastEventID: "evt-2"}},
			"m3": {},
			"m4": nil,
		},
	}}
	specs := []spec.Operator{{Name: "hb", Kind: "missing_heartbeat", Duration: "30s"}, {Name: "other", Kind: "threshold"}}
	timers, err := HeartbeatTimers("dep", "tenant", 1, specs, state)
	if err != nil || len(timers) != 2 {
		t.Fatalf("timers = %+v err=%v", timers, err)
	}
	byKey := map[string]HeartbeatTimer{timers[0].StateKey: timers[0], timers[1].StateKey: timers[1]}
	if !byKey["m1"].DueAt.Equal(processed.Add(30*time.Second)) || !byKey["m2"].DueAt.Equal(last.Add(30*time.Second)) {
		t.Fatalf("due times = %v %v", byKey["m1"].DueAt, byKey["m2"].DueAt)
	}
	if !strings.HasPrefix(byKey["m1"].ID, "tmr_") || byKey["m1"].ID == byKey["m2"].ID || !strings.Contains(string(byKey["m1"].Payload), `"expected_event_id":"evt-1"`) {
		t.Fatalf("timer identity or payload = %+v", byKey["m1"])
	}
	again, err := HeartbeatTimers("dep", "tenant", 1, specs, state)
	if err != nil || len(again) != 2 {
		t.Fatalf("second derivation = %+v err=%v", again, err)
	}
	for _, timer := range again {
		if timer.ID != byKey[timer.StateKey].ID {
			t.Fatalf("timer identity of %s changed between derivations: %s then %s", timer.StateKey, byKey[timer.StateKey].ID, timer.ID)
		}
	}
	bad := []spec.Operator{{Name: "hb", Kind: "missing_heartbeat", Duration: "soon"}}
	if _, err := HeartbeatTimers("dep", "tenant", 1, bad, state); err == nil || !strings.Contains(err.Error(), "parse hb duration") {
		t.Fatalf("err = %v", err)
	}
}
