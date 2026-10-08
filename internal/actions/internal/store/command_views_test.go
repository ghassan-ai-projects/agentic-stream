package store

import "testing"

func TestIntentCommandsReadsOutcomesAndVerificationsWithoutResults(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	for _, statement := range []string{
		`INSERT INTO outcomes (outcome_id, command_id, ordinal, status, outcome_sha256, occurred_at) VALUES ('out-1', '` + commandID + `', 1, 'outcome_unknown', zeroblob(32), '2026-10-08T00:00:00Z')`,
		`DELETE FROM verifications WHERE intent_id = 'int-action'`,
		`INSERT INTO verifications (verification_id, intent_id, command_id, outcome_id, status, updated_at) VALUES ('ver-1', 'int-action', '` + commandID + `', 'out-1', 'awaiting', '2026-10-08T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	commands, err := Reader(db).IntentCommands(t.Context(), "int-action")
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].CommandID != commandID || len(commands[0].Outcomes) != 1 || commands[0].Outcomes[0].ProviderResult != nil || len(commands[0].Verifications) != 1 || commands[0].Verifications[0].Verdict != nil {
		t.Fatalf("intent commands = %+v", commands)
	}
	if Reader(db).Configured() {
		t.Fatal("a reader store can write")
	}
}
