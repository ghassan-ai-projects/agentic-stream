package notify_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var baseTime = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openOutbox(t *testing.T) (*storage.DB, *notify.Service) {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "notify.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	outbox, err := notify.New(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, outbox
}

func appendEvent(ctx context.Context, db *storage.DB, event contractsv1.CloudEvent, now time.Time) (int64, error) {
	var cursor int64
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		cursor, err = notify.Append(ctx, tx, event, now)
		return err
	})
	return cursor, err
}

func countAudits(t *testing.T, db *storage.DB, action string) int {
	t.Helper()
	var audits int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_audits WHERE action = ?", action).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	return audits
}

func TestNewRequiresDatabase(t *testing.T) {
	t.Parallel()
	if outbox, err := notify.New(nil); err == nil || outbox != nil {
		t.Fatalf("New(nil) = %v, %v", outbox, err)
	}
}

func TestNotificationsAreCursorResumableAndAuditExpiredCursor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	for i, id := range []string{"evt-1", "evt-2"} {
		if _, err := appendEvent(ctx, db, testEvent(id, baseTime.Add(time.Duration(i)*time.Second)), baseTime); err != nil {
			t.Fatalf("append %s: %v", id, err)
		}
	}
	page, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10}, baseTime)
	if err != nil || len(page.Records) != 2 || page.Records[0].Cursor != 1 || page.Records[1].Cursor != 2 || page.NextCursor != 2 {
		t.Fatalf("page=%v err=%v", page, err)
	}
	page, err = outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Cursor: 1, Limit: 10}, baseTime)
	if err != nil || len(page.Records) != 1 || page.Records[0].Event.ID != "evt-2" {
		t.Fatalf("resumed page=%v err=%v", page, err)
	}
	if _, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Cursor: -1, Limit: 10}, baseTime); !errors.Is(err, notify.ErrCursorExpired) {
		t.Fatalf("expired cursor error = %v", err)
	}
	if audits := countAudits(t, db, "cursor_expired"); audits != 1 {
		t.Fatalf("expired audits=%d", audits)
	}
}

func TestSlowSubscriberIsAuditedAndDisconnected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	for _, id := range []string{"a", "b", "c"} {
		if _, err := appendEvent(ctx, db, testEvent(id, baseTime), baseTime); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10, MaxLag: 2}, baseTime); !errors.Is(err, notify.ErrSubscriberTooSlow) {
		t.Fatalf("slow subscriber error = %v", err)
	}
	if audits := countAudits(t, db, "subscriber_too_slow"); audits != 1 {
		t.Fatalf("slow audits=%d", audits)
	}
}

func TestReadPageRefusesLimitsOutsideBounds(t *testing.T) {
	t.Parallel()
	_, outbox := openOutbox(t)
	for _, limit := range []int{0, 1001} {
		if _, err := outbox.ReadPage(t.Context(), notify.PageRequest{TenantID: "tenant", Limit: limit}, baseTime); err == nil {
			t.Errorf("limit %d accepted", limit)
		}
	}
}

func TestPoisonNotificationRetriesBeforeAuditedSkip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	if _, err := appendEvent(ctx, db, testEvent("poison", baseTime), baseTime); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE notifications SET event_json = ? WHERE tenant_id = ? AND cursor = 1", []byte("{bad"), "tenant"); err != nil {
		t.Fatal(err)
	}
	request := notify.PageRequest{TenantID: "tenant", Limit: 10}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := outbox.ReadPage(ctx, request, baseTime); !errors.Is(err, notify.ErrNotificationPoison) {
			t.Fatalf("attempt %d error=%v", attempt, err)
		}
	}
	page, err := outbox.ReadPage(ctx, request, baseTime)
	if err != nil || page.Skipped != 1 || page.NextCursor != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if audits := countAudits(t, db, "subscriber_skipped"); audits != 1 {
		t.Fatalf("audits=%d", audits)
	}
}

func TestLifecycleEventsUseStableTypesAndDurableCursors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	events := loadGoldens(t)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for i, event := range events {
			if err := notify.AppendLifecycleEvent(ctx, tx, goldenRequest(t, i, event)); err != nil {
				return fmt.Errorf("append lifecycle event: %w", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	page, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "acme", Limit: 10}, baseTime)
	if err != nil || len(page.Records) != len(events) || page.NextCursor != int64(len(events)) {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	for i, record := range page.Records {
		if record.Cursor != int64(i+1) || record.Event.Type != events[i].Type || record.Event.Source != notify.SourceForTenant("acme") {
			t.Fatalf("record[%d]=%+v", i, record)
		}
	}
}

func TestLifecycleEventRefusesAContractViolation(t *testing.T) {
	t.Parallel()
	db, _ := openOutbox(t)
	request := goldenRequest(t, 0, loadGoldens(t)[0])
	request.Data = map[string]any{"tenant_id": "acme"}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return notify.AppendLifecycleEvent(t.Context(), tx, request) })
	if err == nil {
		t.Fatal("event violating the contract was appended")
	}
}

func TestAppendDuplicatesAndConflictsDoNotConsumeCursors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	original := testEvent("same-id", baseTime)
	for range 2 {
		if cursor, err := appendEvent(ctx, db, original, baseTime); err != nil || cursor != 1 {
			t.Fatalf("duplicate cursor=%d err=%v", cursor, err)
		}
	}
	conflict := original
	conflict.Data = map[string]any{"version": 2}
	var err error
	if conflict.EnvelopeDigest, err = conflict.ComputeEnvelopeDigest(); err != nil {
		t.Fatal(err)
	}
	if _, err := appendEvent(ctx, db, conflict, baseTime); err == nil {
		t.Fatal("conflicting payload accepted")
	}
	if cursor, err := appendEvent(ctx, db, testEvent("next-id", baseTime), baseTime); err != nil || cursor != 2 {
		t.Fatalf("next cursor=%d err=%v", cursor, err)
	}
	page, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10}, baseTime)
	if err != nil || len(page.Records) != 2 || page.Records[0].Event.EnvelopeDigest != original.EnvelopeDigest {
		t.Fatalf("stored notifications changed: page=%+v err=%v", page, err)
	}
}

func TestPruningPreservesHighwaterAndRejectsExpiredEvents(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	original := testEvent("original", baseTime)
	if _, err := appendEvent(ctx, db, original, baseTime); err != nil {
		t.Fatal(err)
	}
	later := baseTime.Add(8 * 24 * time.Hour)
	if deleted, err := outbox.Prune(ctx, later, 7*24*time.Hour); err != nil || deleted != 1 {
		t.Fatalf("prune = %d, %v", deleted, err)
	}
	if _, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "tenant", Limit: 10}, later); !errors.Is(err, notify.ErrCursorExpired) {
		t.Fatalf("old cursor = %v", err)
	}
	if _, err := appendEvent(ctx, db, original, later); !errors.Is(err, notify.ErrEventExpired) {
		t.Fatalf("expired event = %v", err)
	}
	if cursor, err := appendEvent(ctx, db, testEvent("next", later), later); err != nil || cursor != 2 {
		t.Fatalf("next cursor = %d, %v", cursor, err)
	}
}

func TestPruneRefusesRetentionBelowTheFloor(t *testing.T) {
	t.Parallel()
	_, outbox := openOutbox(t)
	if _, err := outbox.Prune(t.Context(), baseTime, 24*time.Hour); err == nil {
		t.Fatal("one-day retention accepted")
	}
}

func testEvent(id string, at time.Time) contractsv1.CloudEvent {
	event := contractsv1.CloudEvent{SpecVersion: "1.0", ID: id, Source: "//agentic-stream/tenant/tenant", Type: "situation.version.published", Subject: "situation/s1", Time: at, DataContentType: "application/json", DataSchema: "urn:situation-runtime:schema:snapshot:v1", Data: map[string]any{"version": 1}, TenantID: "tenant", PartitionKey: "s1", IngestedTime: at, Classification: contractsv1.ClassificationInternal}
	digest, _ := event.ComputeEnvelopeDigest()
	event.EnvelopeDigest = digest
	return event
}

// loadGoldens reads the cross-repository golden events shipped with the contract.
func loadGoldens(t *testing.T) []contractsv1.CloudEvent {
	t.Helper()
	data, err := os.ReadFile("internal/domain/contracts/notification-goldens-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Events []contractsv1.CloudEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document.Events
}

func goldenRequest(t *testing.T, index int, event contractsv1.CloudEvent) notify.LifecycleEvent {
	t.Helper()
	data, ok := event.Data.(map[string]any)
	if !ok {
		t.Fatalf("golden event data is %T", event.Data)
	}
	return notify.LifecycleEvent{
		ID: fmt.Sprintf("lifecycle-%d", index), TenantID: "acme", Type: event.Type, Subject: event.Subject, PartitionKey: event.PartitionKey,
		Data: data, At: baseTime.Add(time.Duration(index) * time.Second),
		Trace: contractsv1.TraceContext{Traceparent: event.Traceparent, Tracestate: event.Tracestate},
	}
}
