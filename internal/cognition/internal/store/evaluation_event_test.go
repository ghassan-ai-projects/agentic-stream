package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
)

func TestEvaluationEventPinsItsIdentityAndInstantText(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	event := evaluationEvent(domain.Evaluation{TriggerID: "trg-1", SituationID: "sit-1", SituationVersion: 2, Outcome: "admitted", EvaluatedAt: at}, "tenant")
	if want := "trg-1:admitted:2026-08-12T12:00:00.000000000Z"; event.ID != want {
		t.Fatalf("event id = %q, want %q: the id is the notification dedupe key", event.ID, want)
	}
	if !event.Time.Equal(at) {
		t.Fatalf("event time = %v", event.Time)
	}
}
