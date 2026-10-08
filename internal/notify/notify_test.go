package notify_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
	payloads := lifecyclePayloads()
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		for i, payload := range payloads {
			if err := notify.AppendLifecycleEvent(ctx, tx, lifecycleRequest(i, payload)); err != nil {
				return fmt.Errorf("append lifecycle event: %w", err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	page, err := outbox.ReadPage(ctx, notify.PageRequest{TenantID: "acme", Limit: 10}, baseTime)
	if err != nil || len(page.Records) != len(payloads) || page.NextCursor != int64(len(payloads)) {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	for i, record := range page.Records {
		data := record.Event.Data.(map[string]any)
		if record.Cursor != int64(i+1) || record.Event.Type != payloads[i].EventType() || record.Event.Source != notify.SourceForTenant("acme") || data["tenant_id"] != "acme" || data["source_authority"] != record.Event.Source {
			t.Fatalf("record[%d]=%+v", i, record)
		}
	}
}

func TestLifecycleEventRefusesAContractViolation(t *testing.T) {
	t.Parallel()
	db, outbox := openOutbox(t)
	request := lifecycleRequest(0, notify.OutcomeRecorded{IntentID: "int_1", Status: "bogus"})
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return notify.AppendLifecycleEvent(t.Context(), tx, request) })
	if err == nil {
		t.Fatal("event violating the contract was appended")
	}
	if page, _ := outbox.ReadPage(t.Context(), notify.PageRequest{TenantID: "acme", Limit: 10}, baseTime); len(page.Records) != 0 {
		t.Fatalf("refused event was stored: %+v", page)
	}
}

func lifecycleRequest(index int, payload notify.Payload) notify.LifecycleEvent {
	return notify.LifecycleEvent{
		ID: fmt.Sprintf("lifecycle-%d", index), TenantID: "acme", Subject: "subject/1", PartitionKey: "partition-1",
		Payload: payload, At: baseTime.Add(time.Duration(index) * time.Second),
	}
}

// lifecyclePayloads is one valid payload per stable lifecycle event type.
func lifecyclePayloads() []notify.Payload {
	digest := "sha256:" + strings.Repeat("1", 64)
	return []notify.Payload{
		notify.OutcomeRecorded{IntentID: "int_1", CommandID: "cmd_1", OutcomeID: "out_1", OutcomeDigest: digest, Status: "succeeded", ReconciliationStatus: "observed"},
		notify.OutcomeReconciled{IntentID: "int_1", CommandID: "cmd_1", OutcomeID: "out_1", OutcomeDigest: digest, FinalStatus: "succeeded", ReconciliationStatus: "reconciled", Verdict: "verified", ReconciliationVersion: 2},
		notify.ApprovalRequested{ApprovalID: "appr_1", IntentID: "int_1", DecisionID: "dec_1", SituationID: "sit_1", SituationVersion: 3, IntentDigest: digest, SnapshotDigest: digest, RiskClass: "R2", ExpiresAt: "2026-08-14T13:00:00Z", Audience: "stream-approval-relay", Summary: "Approval is required", Delta: map[string]any{"phase": "warning"}, Hypothesis: "The motor is degrading", Evidence: []string{"evt_1"}, Action: map[string]any{"target": "motor_1"}, DeclineConsequence: "The intent will not be dispatched."},
		notify.ApprovalWithdrawn{ApprovalID: "appr_1", IntentID: "int_1", SituationID: "sit_1", SituationVersion: 3, Reason: "situation_version_conflict"},
		notify.ApprovalResolved{ApprovalID: "appr_1", IntentID: "int_1", DecisionID: "dec_1", SituationID: "sit_1", SituationVersion: 3, Status: "approved", Reason: "approved"},
		notify.CommandDispatched{CommandID: "cmd_1", IntentID: "int_1", OutcomeID: "out_1", Status: "succeeded"},
		notify.SituationSuperseded{SituationID: "sit_1", SupersededVersion: 2, ReplacementVersion: 3, Reason: "newer_situation_version_admitted"},
		notify.ReconsiderationAdmitted{ReconsiderationID: "rec_1", SituationID: "sit_1", SupersededVersion: 2, CorrectionVersion: 3, InvalidatedCommandID: "cmd_1", InvalidatedOutcomeID: "out_1", TriggerID: "trg_1", SchedulerItemID: "sch_1"},
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

// The three retention steps commit together: if the last one fails, nothing is
// retired and the notification is still readable.
func TestPruneIsOneTransaction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	if _, err := appendEvent(ctx, db, testEvent("kept", baseTime), baseTime); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_tombstone_expiry BEFORE DELETE ON notification_event_tombstones BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO notification_event_tombstones (tenant_id, event_id, event_sha256, retired_at) VALUES ('tenant', 'old', ?, '2000-01-01T00:00:00Z')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	later := baseTime.Add(8 * 24 * time.Hour)
	if _, err := outbox.Prune(ctx, later, 7*24*time.Hour); err == nil {
		t.Fatal("prune succeeded despite the injected failure")
	}
	var notifications, tombstones int
	if err := db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM notifications), (SELECT COUNT(*) FROM notification_event_tombstones)").Scan(&notifications, &tombstones); err != nil {
		t.Fatal(err)
	}
	if notifications != 1 || tombstones != 1 {
		t.Fatalf("after a failed prune: notifications=%d tombstones=%d, want 1 and 1 (nothing retired)", notifications, tombstones)
	}
}

func TestPrunableCountsWithoutRetiring(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, outbox := openOutbox(t)
	if _, err := appendEvent(ctx, db, testEvent("old", baseTime), baseTime); err != nil {
		t.Fatal(err)
	}
	later := baseTime.Add(8 * 24 * time.Hour)
	if count, err := outbox.Prunable(ctx, later, 7*24*time.Hour); err != nil || count != 1 {
		t.Fatalf("prunable = %d, %v", count, err)
	}
	if count, err := outbox.Prunable(ctx, later, 7*24*time.Hour); err != nil || count != 1 {
		t.Fatalf("a count changed the outbox: %d, %v", count, err)
	}
}
