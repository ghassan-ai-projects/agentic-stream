package notify_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestNotificationsAreCursorResumableAndAuditExpiredCursor(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "notify.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	for i, id := range []string{"evt-1", "evt-2"} {
		event := testEvent(id, now.Add(time.Duration(i)*time.Second))
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			_, err := notify.Append(ctx, tx, event, now)
			if err != nil {
				return fmt.Errorf("append notification: %w", err)
			}
			return nil
		}); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
	}
	page, err := notify.ReadPage(ctx, db, "tenant", 0, 10, 0, now)
	if err != nil || len(page.Records) != 2 || page.Records[0].Cursor != 1 || page.Records[1].Cursor != 2 || page.NextCursor != 2 {
		t.Fatalf("page=%v err=%v", page, err)
	}
	page, err = notify.ReadPage(ctx, db, "tenant", 1, 10, 0, now)
	if err != nil || len(page.Records) != 1 || page.Records[0].Event.ID != "evt-2" {
		t.Fatalf("resumed page=%v err=%v", page, err)
	}
	if _, err := notify.ReadPage(ctx, db, "tenant", -1, 10, 0, now); !errors.Is(err, notify.ErrCursorExpired) {
		t.Fatalf("expired cursor error = %v", err)
	}
	var audits int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_audits WHERE action = 'cursor_expired'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("expired audits=%d err=%v", audits, err)
	}
}

func TestPoisonNotificationRetriesBeforeAuditedSkip(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "poison.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	event := testEvent("poison", now)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := notify.Append(ctx, tx, event, now)
		if err != nil {
			return fmt.Errorf("append poison event: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE notifications SET event_json = ? WHERE tenant_id = ? AND cursor = 1", []byte("{bad"), "tenant"); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := notify.ReadPage(ctx, db, "tenant", 0, 10, 0, now); !errors.Is(err, notify.ErrNotificationPoison) {
			t.Fatalf("attempt %d error=%v", attempt, err)
		}
	}
	page, err := notify.ReadPage(ctx, db, "tenant", 0, 10, 0, now)
	if err != nil || page.Skipped != 1 || page.NextCursor != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	var audits int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_audits WHERE action = 'subscriber_skipped'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
}

func TestLifecycleEventsUseStableTypesAndDurableCursors(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	types := []string{
		notify.TypeApprovalRequested, notify.TypeApprovalWithdrawn, notify.TypeApprovalResolved,
		notify.TypeCommandDispatched, notify.TypeOutcomeRecorded, notify.TypeOutcomeReconciled,
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for i, eventType := range types {
			if err := notify.AppendLifecycleEvent(ctx, tx, fmt.Sprintf("lifecycle-%d", i), "tenant", eventType, "subject/1", "partition-1", map[string]any{"index": i}, now.Add(time.Duration(i)*time.Second)); err != nil {
				return fmt.Errorf("append lifecycle event: %w", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	page, err := notify.ReadPage(ctx, db, "tenant", 0, 10, 0, now)
	if err != nil || len(page.Records) != len(types) || page.NextCursor != int64(len(types)) {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	for i, record := range page.Records {
		if record.Cursor != int64(i+1) || record.Event.Type != types[i] || record.Event.Source != notify.SourceForTenant("tenant") {
			t.Fatalf("record[%d]=%+v", i, record)
		}
	}
}

func testEvent(id string, at time.Time) contractsv1.CloudEvent {
	event := contractsv1.CloudEvent{SpecVersion: "1.0", ID: id, Source: "//agentic-stream/tenant/tenant", Type: "situation.version.published", Subject: "situation/s1", Time: at, DataContentType: "application/json", DataSchema: "urn:situation-runtime:schema:snapshot:v1", Data: map[string]any{"version": 1}, TenantID: "tenant", PartitionKey: "s1", IngestedTime: at, Classification: contractsv1.ClassificationInternal}
	digest, _ := event.ComputeEnvelopeDigest()
	event.EnvelopeDigest = digest
	return event
}
