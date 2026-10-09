package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

func TestHeartbeatTimerPinsItsIdentityAndDueInstantText(t *testing.T) {
	t.Parallel()
	last := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	heartbeat := &operators.HeartbeatState{LastEventID: "evt-1", LastEventTime: &last}
	timer, err := newHeartbeatTimer("dep", "tenant", 3, "hb", "motor-1", heartbeat, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	const wantPayload = `{"due_at":"2026-08-12T12:01:00.000000000Z","expected_event_id":"evt-1","operator_id":"hb","state_key":"motor-1"}`
	const wantID = "tmr_95964b241017b60595b29f2580944548ee89b4e3ed76b26ee3b6b81be0fc43e8"
	if string(timer.Payload) != wantPayload || timer.ID != wantID {
		t.Fatalf("timer id %s payload %s: a change here re-arms every pending heartbeat under a new id", timer.ID, timer.Payload)
	}
}

func TestTimerFiringPinsItsFiredInstantText(t *testing.T) {
	t.Parallel()
	feature := operators.Feature{}
	EnrichTimerFeature(&feature, "tenant", 3, DueTimer{ID: "tmr-1"}, time.Date(2026, 8, 12, 12, 1, 0, 0, time.UTC), "physical")
	if got := feature.Metadata["timer_fired_at"]; got != "2026-08-12T12:01:00.000000000Z" {
		t.Fatalf("timer_fired_at = %v", got)
	}
}
