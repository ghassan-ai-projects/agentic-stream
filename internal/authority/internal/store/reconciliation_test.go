package store

import (
	"crypto/sha256"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func deviceState(t *testing.T, device domain.DeviceBoot) domain.DeviceState {
	t.Helper()
	state, err := domain.NewDeviceState(map[string]any{"device_id": device.DeviceID, "boot_id": device.BootID})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func loadRecorded(t *testing.T, db *storage.DB) *domain.Reconciliation {
	t.Helper()
	recorded, err := LoadReconciliation(t.Context(), db, bootA.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func lastResolution(t *testing.T, db *storage.DB) string {
	t.Helper()
	var outcome string
	if err := db.QueryRowContext(t.Context(), `SELECT COALESCE(last_resolution_status, '') FROM device_reconciliation`).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestReconciliationLifecycle(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if loadRecorded(t, db) != nil {
		t.Fatal("unknown device has a reconciliation")
	}
	first := deviceState(t, bootA)
	inTx(t, db, func(tx *sql.Tx) error { return InsertFirstState(t.Context(), tx, first, owner, testNow) })
	if recorded := loadRecorded(t, db); recorded.Device != bootA || recorded.Status != domain.ReconciliationClear || string(recorded.StateSHA256) != string(first.SHA256) {
		t.Fatalf("first state = %+v", recorded)
	}
	inTx(t, db, func(tx *sql.Tx) error { return MarkRequired(t.Context(), tx, bootA, testNow) })
	resolution := domain.Resolution{Device: bootA, Owner: owner, Outcome: domain.ResolutionManualReview, EvidenceJSON: []byte(`{}`), EvidenceSHA256: make([]byte, 32)}
	inTx(t, db, func(tx *sql.Tx) error { return RecordResolution(t.Context(), tx, resolution, testNow) })
	if recorded := loadRecorded(t, db); recorded.Status != domain.ReconciliationRequired || lastResolution(t, db) != "manual_review" {
		t.Fatalf("manual review = %+v", recorded)
	}
	inTx(t, db, func(tx *sql.Tx) error { return RefreshState(t.Context(), tx, first, owner, testNow) })
	if lastResolution(t, db) != "manual_review" {
		t.Fatal("refresh discarded the resolution of the same boot")
	}
	rebooted := deviceState(t, domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-B"})
	inTx(t, db, func(tx *sql.Tx) error { return RecordReboot(t.Context(), tx, rebooted, owner, testNow) })
	if recorded := loadRecorded(t, db); recorded.Device.BootID != "boot-B" || !recorded.Required() || lastResolution(t, db) != "" {
		t.Fatalf("reboot = %+v resolution=%q", recorded, lastResolution(t, db))
	}
}

func TestCountUnresolvedCommandsIsScopedToTheDeviceBoot(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	insertCommand := func(commandID, status string, device domain.DeviceBoot) {
		key := sha256.Sum256([]byte(commandID))
		inTx(t, db, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(t.Context(), `INSERT INTO commands (command_id, intent_id, tenant_id, effector_route,
				normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
				VALUES (?, ?, 'tenant', 'set_indicator', 'fan-01', ?, ?, ?, ?, 'now', 'now')`,
				commandID, "intent-"+commandID, key[:], []byte("{}"), make([]byte, 32), status); err != nil {
				return err
			}
			return InsertBinding(t.Context(), tx, domain.CommandBinding{CommandID: commandID, Target: "fan-01", Device: device, Owner: owner}, testNow)
		})
	}
	insertCommand("cmd-unknown", "outcome_unknown", bootA)
	insertCommand("cmd-done", "succeeded", bootA)
	insertCommand("cmd-other-boot", "reconciling", domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-B"})
	if count, err := CountUnresolvedCommands(t.Context(), db, bootA); err != nil || count != 1 {
		t.Fatalf("unresolved = %d, %v", count, err)
	}
}
