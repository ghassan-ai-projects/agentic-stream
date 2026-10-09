package main

import (
	"strings"
	"testing"
)

func TestSituationListAndShowPrintReadableText(t *testing.T) {
	t.Parallel()
	dbPath := watchRunDatabase(t)
	situationID, entityID := firstSituation(t, dbPath)

	list := runCLI(t, newSituationCommand(), "list", "--db", dbPath)
	for _, want := range []string{situationID, "type=", "entity=", "version=", "phase=", "status=", "latest_event="} {
		if !strings.Contains(list, want) {
			t.Errorf("situation list = %q, want it to contain %q", list, want)
		}
	}
	if none := runCLI(t, newSituationCommand(), "list", "--db", dbPath, "--entity", entityID+"-unknown"); strings.TrimSpace(none) != "situations: none" {
		t.Errorf("situation list of an unknown entity = %q, want %q", none, "situations: none")
	}
	show := runCLI(t, newSituationCommand(), "show", situationID, "--db", dbPath)
	for _, want := range []string{situationID, "phase=", " (from ", "completeness=", "snapshot=sha256:", "evidence=", "\nsnapshot: {"} {
		if !strings.Contains(show, want) {
			t.Errorf("situation show = %q, want it to contain %q", show, want)
		}
	}
}

func TestExplainPrintsDerivationEvidenceAndTriggersAsText(t *testing.T) {
	t.Parallel()
	dbPath := watchRunDatabase(t)
	situationID, _ := firstSituation(t, dbPath)

	situation := runCLI(t, newExplainCommand(), "situation", situationID, "--db", dbPath)
	for _, want := range []string{"fields:\n", "  facts.", " <- ", "evidence:\n", "  #", " source=", "triggers:\n", " outcome=", " threshold="} {
		if !strings.Contains(situation, want) {
			t.Errorf("explain situation = %q, want it to contain %q", situation, want)
		}
	}
	triggerID := triggerIDs(t, dbPath)[0]
	trigger := runCLI(t, newExplainCommand(), "trigger", triggerID, "--db", dbPath)
	for _, want := range []string{triggerID, " outcome=", "\nreasons: ", "\ndelta: ", "\nscheduling: "} {
		if !strings.Contains(trigger, want) {
			t.Errorf("explain trigger = %q, want it to contain %q", trigger, want)
		}
	}
}

func firstSituation(t *testing.T, dbPath string) (situationID, entityID string) {
	t.Helper()
	var situations []struct {
		SituationID string `json:"situation_id"`
		EntityID    string `json:"entity_id"`
	}
	decodeJSON(t, runCLI(t, newSituationCommand(), "list", "--db", dbPath, "--json"), &situations)
	if len(situations) == 0 || situations[0].SituationID == "" || situations[0].EntityID == "" {
		t.Fatalf("the watch trace opened no identifiable Situation: %+v", situations)
	}
	return situations[0].SituationID, situations[0].EntityID
}
