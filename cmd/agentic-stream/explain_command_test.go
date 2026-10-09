package main

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestExplainTracesTheFinalSituationAndEveryTrigger runs the predictive
// maintenance watch trace live, then requires the final Situation version's
// fields to trace to the spec and its evidence to logged events, and every
// trigger evaluation to explain itself with a reason.
func TestExplainTracesTheFinalSituationAndEveryTrigger(t *testing.T) {
	t.Parallel()
	dbPath := watchRunDatabase(t)
	var situations []map[string]any
	decodeJSON(t, runCLI(t, newSituationCommand(), "list", "--db", dbPath, "--json"), &situations)
	if len(situations) == 0 {
		t.Fatal("the watch trace opened no Situation")
	}
	assertFinalVersionExplained(t, dbPath, situations[0]["situation_id"].(string))
	for _, triggerID := range triggerIDs(t, dbPath) {
		var explanation triggerExplanation
		decodeJSON(t, runCLI(t, newExplainCommand(), "trigger", triggerID, "--db", dbPath, "--json"), &explanation)
		if explanation.Evaluation.Outcome == "" || len(explanation.Evaluation.Reasons) == 0 {
			t.Fatalf("trigger %s is not explained: %+v", triggerID, explanation)
		}
	}
}

func assertFinalVersionExplained(t *testing.T, dbPath, situationID string) {
	t.Helper()
	var explanation situationExplanation
	decodeJSON(t, runCLI(t, newExplainCommand(), "situation", situationID, "--db", dbPath, "--json"), &explanation)
	if len(explanation.Fields) == 0 || len(explanation.Version.Evidence) == 0 || len(explanation.Evidence) != len(explanation.Version.Evidence) {
		t.Fatalf("final version is not traced to its evidence: fields=%d lineage=%d logged=%d", len(explanation.Fields), len(explanation.Version.Evidence), len(explanation.Evidence))
	}
	for _, field := range explanation.Fields {
		if field.Derivation.Strategy == "" || field.Derivation.Input == "" {
			t.Fatalf("field %s has no derivation", field.Derivation.Field)
		}
	}
	if len(explanation.Triggers) == 0 {
		t.Fatal("final version has no trigger evaluation")
	}
}

func triggerIDs(t *testing.T, dbPath string) []string {
	t.Helper()
	rows, err := openReadOnly(t, dbPath).QueryContext(t.Context(), "SELECT trigger_id FROM trigger_evaluations")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	return scanStrings(t, rows)
}

func scanStrings(t *testing.T, rows *sql.Rows) []string {
	t.Helper()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		t.Fatal("no rows")
	}
	return values
}

// runCLI executes one command with args and returns what it printed.
func runCLI(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("%s %v: %v", cmd.Name(), args, err)
	}
	return out.String()
}

func decodeJSON(t *testing.T, output string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), into); err != nil {
		t.Fatalf("decode %q: %v", output, err)
	}
}
