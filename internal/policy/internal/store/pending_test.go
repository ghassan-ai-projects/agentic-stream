package store

import (
	"testing"
	"time"
)

func TestTheNextPendingIntentIsTheTenantsOldestUnevaluatedOneAndTiesBreakById(t *testing.T) {
	t.Parallel()
	db, intentID := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	copyIntent(t, db, intentID, "int-foreign", "tenant_id = 'other', created_at = '2026-08-01T00:00:00Z'")
	copyIntent(t, db, intentID, "int-evaluated", "policy_status = 'approved', created_at = '2026-08-02T00:00:00Z'")
	copyIntent(t, db, intentID, "int-older", "created_at = '2026-08-11T00:00:00Z'")
	copyIntent(t, db, intentID, "int-a-tie", "created_at = '2026-08-12T00:00:00Z'")

	for _, want := range []string{"int-older", "int-a-tie", intentID} {
		next, found, err := NextPendingIntent(t.Context(), db.DB, "tenant")
		if err != nil || !found || next != want {
			t.Fatalf("next pending = %q %t %v, want %q", next, found, err, want)
		}
		if _, err := db.ExecContext(t.Context(), "UPDATE intents SET policy_status = 'denied' WHERE intent_id = ?", want); err != nil {
			t.Fatal(err)
		}
	}
	if next, found, err := NextPendingIntent(t.Context(), db.DB, "tenant"); err != nil || found || next != "" {
		t.Fatalf("after all are evaluated: %q %t %v, want an empty queue", next, found, err)
	}
	if next, found, err := NextPendingIntent(t.Context(), db.DB, "other"); err != nil || !found || next != "int-foreign" {
		t.Fatalf("the other tenant's queue: %q %t %v", next, found, err)
	}
}

func TestReadingTheNextPendingIntentFromAClosedDatabaseFails(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	_ = db.Close()
	if _, _, err := NextPendingIntent(t.Context(), db.DB, "tenant"); err == nil {
		t.Fatal("a closed database reported a queue")
	}
}
