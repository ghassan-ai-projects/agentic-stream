package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func priorFixture() InvalidatedCommand {
	return InvalidatedCommand{
		CommandID: "c", DecisionID: "d", OutcomeID: "o", IntentID: "i", IntentType: "ticket", RiskClass: "R1",
		CommandStatus: "succeeded", OutcomeStatus: "succeeded", ReconciliationStatus: "observed", OutcomeOrdinal: 2,
		DecisionJSON: []byte(`{"decision_id":"d"}`), CommandJSON: []byte(`{"command_id":"c","payload":{"p":1}}`),
		ProviderJSON: []byte(`{"accepted":true}`), ObservedJSON: []byte(`{"state":"off"}`), OutcomeSHA: make([]byte, 32),
	}
}

func TestPriorOutcomeDocumentKeys(t *testing.T) {
	t.Parallel()
	outcome := priorFixture().PriorOutcome()
	for _, key := range []string{"outcome_id", "command_id", "ordinal", "status", "reconciliation_status", "outcome_sha256", "provider_result", "observed_effect"} {
		if _, ok := outcome[key]; !ok {
			t.Fatalf("prior outcome lacks %s: %#v", key, outcome)
		}
	}
}

func TestReconsiderationEvidenceCarriesEveryPriorDocument(t *testing.T) {
	t.Parallel()
	r := NewReconsideration(situations.Version{SituationID: "s", Version: 3, PreviousVersion: 2}, priorFixture())
	encoded, err := r.EvidenceJSON(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatal(err)
	}
	decision, command, outcome := evidence["prior_decision"].(map[string]any), evidence["prior_command"].(map[string]any), evidence["prior_outcome"].(map[string]any)
	if decision["decision_id"] != "d" || command["parameters"] == nil || outcome["command_id"] != "c" || evidence["reconsideration_id"] != r.ID {
		t.Fatalf("prior documents: %s", encoded)
	}
}

func TestPriorDocumentsRejectIdentityBeforeCommand(t *testing.T) {
	t.Parallel()
	command := priorFixture()
	command.DecisionJSON = []byte(`{"decision_id":"other"}`)
	command.CommandJSON = []byte(`invalid`)
	_, err := command.priorDocuments()
	if err == nil || !strings.HasPrefix(err.Error(), "decision_id identity mismatch:") {
		t.Fatalf("error precedence: %v", err)
	}
}
