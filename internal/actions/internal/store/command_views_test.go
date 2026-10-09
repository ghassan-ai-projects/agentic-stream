package store

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func seedOutcomeAndVerification(t *testing.T, db *storage.DB, commandID, providerResult string) {
	t.Helper()
	execute(t, db, `INSERT INTO outcomes (outcome_id, command_id, ordinal, status, provider_result_json, outcome_sha256, occurred_at)
		VALUES ('out-2', ?, 2, 'succeeded', CAST(? AS BLOB), zeroblob(32), '2026-10-08T00:00:02Z')`, commandID, providerResult)
	execute(t, db, `INSERT INTO outcomes (outcome_id, command_id, ordinal, status, outcome_sha256, occurred_at)
		VALUES ('out-1', ?, 1, 'unknown', zeroblob(32), '2026-10-08T00:00:01Z')`, commandID)
	execute(t, db, `DELETE FROM verifications WHERE intent_id = 'int-action'`)
	execute(t, db, `INSERT INTO verifications (verification_id, intent_id, command_id, outcome_id, status, updated_at)
		VALUES ('ver-1', 'int-action', ?, 'out-2', 'awaiting', '2026-10-08T00:00:02Z')`, commandID)
}

func TestIntentCommandsReadTheirOutcomesInOrderWithTheirProviderResults(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	seedOutcomeAndVerification(t, db, commandID, `{"accepted":true}`)
	commands, err := Reader(db).IntentCommands(t.Context(), "int-action")
	if err != nil || len(commands) != 1 || commands[0].CommandID != commandID {
		t.Fatalf("commands = %+v, %v; want the intent's one command", commands, err)
	}
	outcomes := commands[0].Outcomes
	if len(outcomes) != 2 || outcomes[0].OutcomeID != "out-1" || outcomes[1].OutcomeID != "out-2" || outcomes[0].ProviderResult != nil || string(outcomes[1].ProviderResult) != `{"accepted":true}` {
		t.Fatalf("outcomes = %+v, want ordinal order, the first without and the second with its provider result", outcomes)
	}
	if verifications := commands[0].Verifications; len(verifications) != 1 || verifications[0].Status != "awaiting" || verifications[0].Verdict != nil {
		t.Fatalf("verifications = %+v, want one awaiting verification without a verdict", verifications)
	}
}

func TestAnIntentWithoutCommandsReadsAsEmptyAndAnotherIntentsCommandsAreNotRead(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	if commands, err := Reader(db).IntentCommands(t.Context(), "int-other"); err != nil || len(commands) != 0 {
		t.Fatalf("commands = %+v, %v; want none", commands, err)
	}
}
