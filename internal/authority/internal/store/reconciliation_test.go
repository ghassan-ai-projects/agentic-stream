package store

import (
	"crypto/sha256"
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

func loadRecorded(t *testing.T, s *Store) *domain.Reconciliation {
	t.Helper()
	recorded, err := s.LoadReconciliation(t.Context(), bootA.DeviceID)
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
	s, db := openStore(t)
	if loadRecorded(t, s) != nil {
		t.Fatal("unknown device has a reconciliation")
	}
	first := deviceState(t, bootA)
	work(t, s, func(tx *Tx) error { return tx.InsertFirstState(t.Context(), first, owner, testNow) })
	if recorded := loadRecorded(t, s); recorded.Device != bootA || recorded.Status != domain.ReconciliationClear || string(recorded.StateSHA256) != string(first.SHA256) {
		t.Fatalf("first state = %+v", recorded)
	}
	resolution := domain.Resolution{Device: bootA, Owner: owner, Outcome: domain.ResolutionManualReview, EvidenceJSON: []byte(`{}`), EvidenceSHA256: make([]byte, 32)}
	work(t, s, func(tx *Tx) error {
		if err := tx.MarkRequired(t.Context(), bootA, testNow); err != nil {
			return err
		}
		return tx.RecordResolution(t.Context(), resolution, testNow)
	})
	if recorded := loadRecorded(t, s); recorded.Status != domain.ReconciliationRequired || lastResolution(t, db) != "manual_review" {
		t.Fatalf("manual review = %+v", recorded)
	}
	work(t, s, func(tx *Tx) error { return tx.RefreshState(t.Context(), first, owner, testNow) })
	if lastResolution(t, db) != "manual_review" {
		t.Fatal("refresh discarded the resolution of the same boot")
	}
	rebooted := deviceState(t, domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-B"})
	work(t, s, func(tx *Tx) error { return tx.RecordReboot(t.Context(), rebooted, owner, testNow) })
	var recorded *domain.Reconciliation
	work(t, s, func(tx *Tx) error {
		var err error
		recorded, err = tx.LoadReconciliation(t.Context(), bootA.DeviceID)
		return err
	})
	if recorded.Device.BootID != "boot-B" || !recorded.Required() || lastResolution(t, db) != "" {
		t.Fatalf("reboot = %+v", recorded)
	}
}

func TestCountUnresolvedCommandsIsScopedToTheDeviceBoot(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	insertCommand := func(commandID, status string, device domain.DeviceBoot) {
		key := sha256.Sum256([]byte(commandID))
		if _, err := db.ExecContext(t.Context(), `INSERT INTO commands (command_id, intent_id, tenant_id, effector_route,
			normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
			VALUES (?, ?, 'tenant', 'set_indicator', 'fan-01', ?, ?, ?, ?, 'now', 'now')`,
			commandID, "intent-"+commandID, key[:], []byte("{}"), make([]byte, 32), status); err != nil {
			t.Fatal(err)
		}
		work(t, s, func(tx *Tx) error {
			return tx.InsertBinding(t.Context(), domain.CommandBinding{CommandID: commandID, Target: "fan-01", Device: device, Owner: owner}, testNow)
		})
	}
	insertCommand("cmd-unknown", "outcome_unknown", bootA)
	insertCommand("cmd-done", "succeeded", bootA)
	insertCommand("cmd-other-boot", "reconciling", domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-B"})
	work(t, s, func(tx *Tx) error {
		if count, err := tx.CountUnresolvedCommands(t.Context(), bootA); err != nil || count != 1 {
			t.Fatalf("unresolved = %d, %v", count, err)
		}
		return nil
	})
}
