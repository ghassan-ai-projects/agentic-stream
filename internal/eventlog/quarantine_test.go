package eventlog_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestQuarantineIsBoundedAndReleasable(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "quarantine.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	log := eventlog.NewEventLog(db)
	event := map[string]any{
		"id": "evt-poison", "type": "sensor.temperature", "schema_version": "1.0", "source": "test",
		"data": map[string]any{"unexpected": true},
	}
	for i := 0; i < 12; i++ {
		if err := log.Quarantine(ctx, "tenant-1", event, "unknown_payload_field", "2026-08-12T12:00:00Z"); err != nil {
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
	if err := log.ReleaseQuarantine(ctx, "tenant-1", "evt-poison", "2026-08-12T12:01:00Z"); err != nil {
		t.Fatalf("release: %v", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM event_quarantine WHERE tenant_id = ? AND event_id = ?", "tenant-1", "evt-poison").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "released" {
		t.Fatalf("status = %q, want released", status)
	}
}

func TestRecordGapPreservesDiscontinuity(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "gap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	log := eventlog.NewEventLog(db)
	if err := log.RecordGap(ctx, "gap-1", "tenant-1", 2, 10, 12, "quarantine_overflow", "2026-08-12T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var reason string
	if err := db.QueryRowContext(ctx, "SELECT reason_code FROM event_gaps WHERE gap_id = ?", "gap-1").Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "quarantine_overflow" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestQuarantineRejectsEventIDHashConflict(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "conflict.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	log := eventlog.NewEventLog(db)
	base := map[string]any{"id": "evt-conflict", "type": "sensor.temperature", "schema_version": "1.0", "source": "test", "data": map[string]any{"value": 1}}
	if err := log.Quarantine(ctx, "tenant-1", base, "invalid", "2026-08-12T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	base["data"] = map[string]any{"value": 2}
	if err := log.Quarantine(ctx, "tenant-1", base, "invalid", "2026-08-12T12:01:00Z"); err == nil {
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
