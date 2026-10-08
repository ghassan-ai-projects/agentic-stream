package eventlog_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestQuarantineIsBoundedAndReleasable(t *testing.T) {
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	log := eventlog.NewEventLog(db)
	poison := []byte(`{"id":"evt-poison","data":{"unexpected":true}}`)
	for i := 0; i < 12; i++ {
		if err := log.QuarantineRaw(ctx, "tenant-1", "evt-poison", poison, "unknown_payload_field", "2026-08-12T12:00:00Z"); err != nil {
			t.Fatalf("quarantine %d: %v", i, err)
		}
	}
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT attempt_count FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", "tenant-1", "evt-poison").Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 10 {
		t.Fatalf("attempt count = %d, want bounded count 10", attempts)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", "tenant-1", "evt-poison").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("status = %q, want rejected after retry exhaustion", status)
	}
	var gaps int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_gaps WHERE tenant_id = ? AND reason_code = 'quarantine_retry_exhausted'", "tenant-1").Scan(&gaps); err != nil {
		t.Fatal(err)
	}
	if gaps != 1 {
		t.Fatalf("overflow gaps = %d, want 1", gaps)
	}

	if err := log.QuarantineRaw(ctx, "tenant-1", "evt-releasable", []byte(`{"id":"evt-releasable"}`), "unknown_payload_field", "2026-08-12T12:00:00Z"); err != nil {
		t.Fatalf("releasable quarantine: %v", err)
	}
	if err := log.ReleaseQuarantine(ctx, "tenant-1", "evt-releasable", "2026-08-12T12:01:00Z"); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestQuarantineRejectsEventIDHashConflict(t *testing.T) {
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	log := eventlog.NewEventLog(db)
	if err := log.QuarantineRaw(ctx, "tenant-1", "evt-conflict", []byte(`{"value":1}`), "invalid", "2026-08-12T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := log.QuarantineRaw(ctx, "tenant-1", "evt-conflict", []byte(`{"value":2}`), "invalid", "2026-08-12T12:01:00Z"); err == nil {
		t.Fatal("expected hash conflict")
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", "tenant-1", "evt-conflict").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("status = %q, want rejected", status)
	}
}
