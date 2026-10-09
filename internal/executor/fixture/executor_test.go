package fixture_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
)

func TestFixtureExecutorConformsToTheExecutorPort(t *testing.T) {
	t.Parallel()
	if err := executorconformance.Run(t.Context(), fixture.New()); err != nil {
		t.Fatal(err)
	}
}

func fixtureRequest(entity, requestJSON string) *episodes.Request {
	return &episodes.Request{
		EpisodeID: "episode", AttemptID: "attempt", Fence: 3, TenantID: "tenant", SituationID: "sit", SituationVersion: 2,
		SnapshotSHA256: "sha256:bound", EntityID: entity, RequestJSON: []byte(requestJSON),
	}
}

func TestFixtureProposesATicketForTheSnapshotPhaseAndTrigger(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		entity     string
		requestRaw string
		wantPhase  string
		wantTrig   string
		wantEntity bool
	}{
		{"bound snapshot", "motor", `{"snapshot":{"phase":"warning"},"trigger":{"trigger_name":"temperature"}}`, "warning", "temperature", true},
		{"missing snapshot and trigger", "", `{}`, "unknown", "unknown", false},
		{"fields of another type", "motor", `{"snapshot":{"phase":7},"trigger":{"trigger_name":false}}`, "unknown", "unknown", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := fixtureRequest(tt.entity, tt.requestRaw)
			executor := fixture.New()
			outcome, err := executor.Execute(t.Context(), req)
			if err != nil {
				t.Fatalf("Execute() = %v", err)
			}
			if outcome.Status != "produced" || outcome.AttemptID != req.AttemptID || outcome.Fence != req.Fence || len(outcome.Reasons) != 1 || outcome.Reasons[0] != "deterministic fake outcome" {
				t.Fatalf("Execute() outcome = %+v", outcome)
			}
			document := decodeDecision(t, outcome)
			intent := document["intents"].([]any)[0].(map[string]any)
			parameters := intent["parameters"].(map[string]any)
			wantSummary := "fake decision for phase " + tt.wantPhase + " via trigger " + tt.wantTrig
			if document["summary"] != wantSummary || parameters["reason"] != tt.wantPhase || (parameters["entity_id"] == tt.entity) != tt.wantEntity {
				t.Fatalf("decision summary %q, intent parameters %v; want %q for phase %q, entity bound: %v", document["summary"], parameters, wantSummary, tt.wantPhase, tt.wantEntity)
			}
			if intent["type"] != "create_maintenance_ticket" || intent["risk_class"] != "R1" || intent["intent_id"] != "int_episode" || !contractsv1.VerifyIntentDigest(intent) {
				t.Fatalf("intent = %v, want a digest-bound R1 maintenance ticket", intent)
			}
			again, err := executor.Execute(t.Context(), req)
			if err != nil || !bytes.Equal(outcome.DecisionJSON, again.DecisionJSON) {
				t.Fatalf("second Execute() = %v, %v; want identical decision bytes", again, err)
			}
		})
	}
}

func TestFixtureDecisionPreservesIdentityAndDigest(t *testing.T) {
	t.Parallel()
	req := fixtureRequest("motor", `{}`)
	outcome, err := fixture.New().Execute(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	document := decodeDecision(t, outcome)
	if !canonicaljson.Verify(canonicaljson.DomainDecision, document, outcome.DecisionSHA256) {
		t.Fatal("the outcome digest does not match its decision")
	}
	want := map[string]any{
		"decision_id": "dec_episode", "episode_id": "episode", "attempt_id": "attempt", "fence": float64(3),
		"snapshot_digest": "sha256:bound", "situation_id": "sit", "situation_version": float64(2),
	}
	for field, value := range want {
		if document[field] != value {
			t.Errorf("decision %s = %v, want %v", field, document[field], value)
		}
	}
}

func TestFixtureRefusesAMalformedRequest(t *testing.T) {
	t.Parallel()
	outcome, err := fixture.New().Execute(t.Context(), fixtureRequest("", "{"))
	if outcome != nil || err == nil || !strings.Contains(err.Error(), "unmarshal request") {
		t.Fatalf("Execute() = %v, %v; want an unmarshal error and no outcome", outcome, err)
	}
}

func decodeDecision(t *testing.T, outcome *episodes.Outcome) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(outcome.DecisionJSON, &document); err != nil {
		t.Fatalf("decision is not json: %v", err)
	}
	return document
}
