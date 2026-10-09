package store

import (
	"crypto/sha256"
	"database/sql"
	"testing"
)

func insertBareCommand(t *testing.T, tx *sql.Tx, commandID, status, tenant string) {
	t.Helper()
	key := sha256.Sum256([]byte(commandID))
	updatedAt := "2026-10-06T12:00:0" + commandID[len(commandID)-1:] + ".000000000Z"
	if _, err := tx.ExecContext(t.Context(), `INSERT INTO commands (command_id, intent_id, tenant_id, effector_route,
		normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		VALUES (?, ?, ?, 'set_indicator', 'fan-01', ?, ?, ?, ?, '2026-10-06T12:00:00.000000000Z', ?)`,
		commandID, "intent-"+commandID, tenant, key[:], []byte("{}"), make([]byte, 32), status, updatedAt); err != nil {
		t.Fatalf("insert command %s: %v", commandID, err)
	}
}

func TestOnlyCommandsAwaitingReconciliationAreCountedAsUnresolved(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	execute(t, db, `PRAGMA foreign_keys = OFF`)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		for commandID, status := range map[string]string{"cmd-1": "outcome_unknown", "cmd-2": "reconciling", "cmd-3": "manual_review", "cmd-4": "succeeded"} {
			insertBareCommand(t, tx, commandID, status, "tenant")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		ids  []string
		want int64
	}{
		{"no ids", nil, 0},
		{"a finished command", []string{"cmd-4"}, 0},
		{"an unknown ID is ignored", []string{"cmd-1", "cmd-4", "cmd-missing"}, 1},
		{"every unresolved status", []string{"cmd-1", "cmd-2", "cmd-3"}, 3},
	}
	for _, tc := range cases {
		var got int64
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			var err error
			got, err = JoinCaller(tx).CountUnresolvedOutcomes(t.Context(), tc.ids)
			return err
		}); err != nil || got != tc.want {
			t.Errorf("%s: unresolved = %d, %v; want %d", tc.name, got, err, tc.want)
		}
	}
}

func TestTheReconciliationQueueListsAnUnresolvedCommandOfTheTenantOldestFirst(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	execute(t, db, `PRAGMA foreign_keys = OFF`)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		insertBareCommand(t, tx, "cmd-3", "reconciling", "tenant")
		insertBareCommand(t, tx, "cmd-1", "manual_review", "tenant")
		insertBareCommand(t, tx, "cmd-2", "succeeded", "tenant")
		insertBareCommand(t, tx, "cmd-5", "reconciling", "other-tenant")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	awaiting, err := newStore(db).AwaitingReconciliation(t.Context(), "tenant")
	if err != nil || len(awaiting) != 2 || awaiting[0].CommandID != "cmd-1" || awaiting[1].CommandID != "cmd-3" {
		t.Fatalf("awaiting = %+v, %v; want cmd-1 then cmd-3 and nothing from another tenant or a finished command", awaiting, err)
	}
	if awaiting[0].Status != "manual_review" || awaiting[0].IntentID != "intent-cmd-1" || awaiting[0].Route != "set_indicator" || awaiting[0].Target != "fan-01" {
		t.Fatalf("awaiting[0] = %+v", awaiting[0])
	}
}
