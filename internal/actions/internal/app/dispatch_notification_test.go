package app_test

import (
	"encoding/json"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestASuccessfulDispatchPublishesItsRecordedAndReconciledOutcome(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	dispatchOnce(t, newDispatcher(t, db, succeeds()))

	outcomeID := queryString(t, db, "SELECT outcome_id FROM outcomes WHERE command_id = ?", commandID)
	outcomeSHA := queryString(t, db, "SELECT lower(hex(outcome_sha256)) FROM outcomes WHERE command_id = ?", commandID)
	wantDigest := "sha256:" + outcomeSHA
	assertNotificationData(t, db, notify.OutcomeRecorded{}.EventType(), map[string]any{
		"outcome_id": outcomeID, "command_id": commandID, "intent_id": "int-action", "status": "succeeded",
		"reconciliation_status": "observed", "outcome_digest": wantDigest,
	})
	assertNotificationData(t, db, notify.OutcomeReconciled{}.EventType(), map[string]any{
		"outcome_id": outcomeID, "command_id": commandID, "intent_id": "int-action", "final_status": "succeeded",
		"reconciliation_status": "reconciled", "verdict": "verified", "reconciliation_version": float64(1),
		"source_authority": notify.SourceForTenant("tenant"),
	})
	assertSourceAuthorityMatchesEnvelope(t, db, notify.OutcomeRecorded{}.EventType())
	assertSourceAuthorityMatchesEnvelope(t, db, notify.OutcomeReconciled{}.EventType())
}

func TestAnUnknownOutcomeIsPublishedAsRecordedAndNeverAsReconciled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		effector *scriptedEffector
	}{
		{"unknown outcome", failsWith(unknownOutcome)},
		{"accepted transport awaiting verification", pendingVerification()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _ := openActionFixture(t)
			dispatchOnce(t, newDispatcher(t, db, tc.effector))
			assertNotificationData(t, db, notify.OutcomeRecorded{}.EventType(), map[string]any{"status": "unknown", "reconciliation_status": "required"})
			if n := count(t, db, "SELECT COUNT(*) FROM notifications WHERE event_type = ?", notify.OutcomeReconciled{}.EventType()); n != 0 {
				t.Fatalf("%d outcome.reconciled notifications before any independent evidence", n)
			}
		})
	}
}

func assertNotificationData(t *testing.T, db *storage.DB, eventType string, want map[string]any) {
	t.Helper()
	data := readNotificationData(t, db, eventType)
	for key, wantValue := range want {
		if data[key] != wantValue {
			t.Errorf("%s %s = %v, want %v (payload %v)", eventType, key, data[key], wantValue, data)
		}
	}
}

func assertSourceAuthorityMatchesEnvelope(t *testing.T, db *storage.DB, eventType string) {
	t.Helper()
	event := readNotification(t, db, eventType)
	data := readNotificationData(t, db, eventType)
	if authority, _ := data["source_authority"].(string); authority != event.Source {
		t.Fatalf("%s source_authority = %q, envelope source = %q", eventType, authority, event.Source)
	}
}

func readNotificationData(t *testing.T, db *storage.DB, eventType string) map[string]any {
	t.Helper()
	event := readNotification(t, db, eventType)
	data, ok := event.Data.(map[string]any)
	if !ok {
		t.Fatalf("%s notification data type = %T, want map[string]any", eventType, event.Data)
	}
	return data
}

func readNotification(t *testing.T, db *storage.DB, eventType string) contractsv1.CloudEvent {
	t.Helper()
	var eventJSON []byte
	if err := db.QueryRowContext(t.Context(), "SELECT event_json FROM notifications WHERE event_type = ? ORDER BY cursor LIMIT 1", eventType).Scan(&eventJSON); err != nil {
		t.Fatalf("read %s notification: %v", eventType, err)
	}
	var event contractsv1.CloudEvent
	if err := json.Unmarshal(eventJSON, &event); err != nil {
		t.Fatalf("decode %s notification: %v", eventType, err)
	}
	return event
}
