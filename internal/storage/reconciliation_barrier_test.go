package storage_test

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestReconciliationOpeningRollsBackWithoutAudit(t *testing.T) {
	t.Parallel()
	db, _ := openOwnerDB(t)
	authority := &storage.TargetAuthority{DB: db}
	store := &storage.ReconciliationStore{DB: db, Authority: authority}
	state := map[string]any{"device_id": "device", "boot_id": "boot"}
	if _, err := store.BindState(t.Context(), state, "epoch", "instance"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER reject_barrier_audit
		BEFORE INSERT ON device_authority_events WHEN NEW.event_type = 'reconciliation_opened'
		BEGIN SELECT RAISE(ABORT, 'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.Require(t.Context(), "device", "boot", "epoch", "instance", "unknown receipt"); err == nil || !strings.Contains(err.Error(), "record authority event") {
		t.Fatalf("opening failure = %v", err)
	}
	if required, err := store.Required(t.Context(), "device"); err != nil || required {
		t.Fatalf("barrier opened without its audit: required=%v err=%v", required, err)
	}
}

func TestReconciliationOpeningRejectsPreviousBootAfterAuthorityLoss(t *testing.T) {
	t.Parallel()
	db, _ := openOwnerDB(t)
	store := &storage.ReconciliationStore{DB: db, Authority: &storage.TargetAuthority{DB: db}}
	if _, err := store.BindState(t.Context(), map[string]any{"device_id": "device", "boot_id": "current"}, "epoch", "instance"); err != nil {
		t.Fatal(err)
	}
	if err := store.RequireAfterAuthorityLoss(t.Context(), "device", "old", "epoch", "instance", "unknown receipt"); err == nil || !strings.Contains(err.Error(), "does not match current device boot") {
		t.Fatalf("old boot opening = %v", err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM device_authority_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected opening recorded an audit: count=%d err=%v", count, err)
	}
}
