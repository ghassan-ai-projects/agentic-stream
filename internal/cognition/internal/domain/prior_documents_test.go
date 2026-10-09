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

func TestPriorDocumentsRefuseUnreadableOrMismatchedHistory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*InvalidatedCommand)
		wantErr string
	}{
		{"decision missing", func(c *InvalidatedCommand) { c.DecisionJSON = nil }, "prior decision is empty"},
		{"decision not json", func(c *InvalidatedCommand) { c.DecisionJSON = []byte(`{`) }, "decode prior decision"},
		{"decision not an object", func(c *InvalidatedCommand) { c.DecisionJSON = []byte(`null`) }, "prior decision must be an object"},
		{"decision without identity", func(c *InvalidatedCommand) { c.DecisionID = "" }, "decision_id is required"},
		{"command missing", func(c *InvalidatedCommand) { c.CommandJSON = nil }, "executed command is empty"},
		{"command for another command", func(c *InvalidatedCommand) { c.CommandJSON = []byte(`{"command_id":"other"}`) }, "command_id identity mismatch"},
		{"command for another intent", func(c *InvalidatedCommand) { c.CommandJSON = []byte(`{"command_id":"c","intent_id":"other"}`) }, "intent_id identity mismatch"},
		{"command without intent identity", func(c *InvalidatedCommand) { c.IntentID = "" }, "intent_id is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			command := priorFixture()
			tc.mutate(&command)
			if _, err := command.priorDocuments(); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestPriorCommandCarriesItsStatusAndKeepsAnExistingParameterSet(t *testing.T) {
	t.Parallel()
	command := priorFixture()
	documents, err := command.priorDocuments()
	if err != nil {
		t.Fatal(err)
	}
	if documents.command["status"] != "succeeded" || documents.command["intent_type"] != "ticket" || documents.command["risk_class"] != "R1" || documents.command["intent_id"] != "i" {
		t.Fatalf("command = %v", documents.command)
	}
	if payload, ok := documents.command["parameters"].(map[string]any); !ok || payload["p"] != 1.0 {
		t.Fatalf("parameters = %v, want the payload copied", documents.command["parameters"])
	}
	command.CommandJSON = []byte(`{"command_id":"c","payload":{"p":1},"parameters":{"q":2}}`)
	documents, err = command.priorDocuments()
	if err != nil || documents.command["parameters"].(map[string]any)["q"] != 2.0 {
		t.Fatalf("explicit parameters were replaced: %v, %v", documents.command, err)
	}
}

func TestPriorOutcomeOmitsResultsTheProviderNeverReturned(t *testing.T) {
	t.Parallel()
	command := priorFixture()
	command.ProviderJSON, command.ObservedJSON = nil, nil
	outcome := command.PriorOutcome()
	if _, ok := outcome["provider_result"]; ok {
		t.Error("an absent provider result was invented")
	}
	if _, ok := outcome["observed_effect"]; ok {
		t.Error("an absent observed effect was invented")
	}
	if outcome["ordinal"] != 2 || outcome["status"] != "succeeded" || outcome["reconciliation_status"] != "observed" {
		t.Fatalf("outcome = %v", outcome)
	}
}
