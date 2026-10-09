package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func TestProducedOutcomeSealsTheDecisionForTheAttempt(t *testing.T) {
	t.Parallel()

	outcome, err := (&Request{AttemptID: "att", Fence: 3}).ProducedOutcome(map[string]any{"decision_id": "d-1"}, 9)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != string(episodeledger.AttemptProduced) || outcome.AttemptID != "att" || outcome.Fence != 3 || outcome.CostMicrounits != 9 {
		t.Fatalf("outcome = %+v", outcome)
	}
	if !canonicaljson.Verify(canonicaljson.DomainDecision, map[string]any{"decision_id": "d-1"}, outcome.DecisionSHA256) || string(outcome.DecisionJSON) != `{"decision_id":"d-1"}` {
		t.Fatalf("decision = %s %s", outcome.DecisionJSON, outcome.DecisionSHA256)
	}
}
