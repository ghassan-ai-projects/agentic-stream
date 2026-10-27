package workerfake

import (
	"encoding/json"
	"strings"
	"testing"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func proposeRequest(snapshot string) *runtimev1.EpisodeRequest {
	return &runtimev1.EpisodeRequest{EpisodeId: "ep-1", AttemptId: "at-1", Fence: 1, TenantId: "default", SituationId: "sit-1", SituationVersion: 2,
		SnapshotSha256: make([]byte, 32), SnapshotJson: []byte(snapshot)}
}

func proposed(t *testing.T, req *runtimev1.EpisodeRequest, answer Answer) ([]*runtimev1.EpisodeEvent, error) {
	t.Helper()
	var events []*runtimev1.EpisodeEvent
	err := Propose(req, func(event *runtimev1.EpisodeEvent) error {
		events = append(events, event)
		return nil
	}, answer)
	return events, err
}

func TestProposeStreamsABudgetTheDecisionAndAProducedTerminal(t *testing.T) {
	t.Parallel()
	events, err := proposed(t, proposeRequest(`{"entity":{"id":"pump-3"}}`), Answer{Hypothesis: "wear", DiagnosisCode: "bearing_wear",
		Intent: &ProposedIntent{Type: "schedule_maintenance", Risk: "R1", Parameters: map[string]any{"work_order": "bearing_wear"}}})
	if err != nil || len(events) != 3 || events[2].GetTerminal().GetStatus() != runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED {
		t.Fatalf("events = %v (%v), want budget, decision and a produced terminal", events, err)
	}
	var decision map[string]any
	if err := json.Unmarshal(events[1].GetDecision().GetDecisionJson(), &decision); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(decision)
	if decision["diagnosis_code"] != "bearing_wear" || !strings.Contains(string(encoded), `"entity_id":"pump-3"`) || !strings.Contains(string(encoded), `"intent_digest":"sha256:`) {
		t.Fatalf("decision = %s, want the diagnosis and a digested intent for the snapshot's entity", encoded)
	}
}

func TestProposeWithoutIntentNeedsNoEntity(t *testing.T) {
	t.Parallel()
	events, err := proposed(t, proposeRequest(`{}`), Answer{Hypothesis: "none"})
	if err != nil || len(events) != 3 {
		t.Fatalf("events = %v (%v), want a decision without intents", events, err)
	}
}

func TestProposeRefusesAnIntentWithoutTheSnapshotsEntity(t *testing.T) {
	t.Parallel()
	for snapshot, want := range map[string]string{`{}`: "snapshot has no entity id", `{`: "decode the snapshot"} {
		_, err := proposed(t, proposeRequest(snapshot), Answer{Intent: &ProposedIntent{Type: "x", Risk: "R1"}})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("snapshot %s: %v, want %q", snapshot, err, want)
		}
	}
}
