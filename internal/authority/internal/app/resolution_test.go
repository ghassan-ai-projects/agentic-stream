package app_test

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

// insertUnknownOutcome records a command bound to device whose outcome the
// action ledger still treats as unknown.
func (f *fixture) insertUnknownOutcome(t *testing.T, commandID string, device domain.DeviceBoot) {
	t.Helper()
	if _, err := f.db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(commandID))
	if _, err := f.db.ExecContext(t.Context(), `INSERT INTO commands (command_id, intent_id, tenant_id, effector_route,
		normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		VALUES (?, ?, 'tenant', 'set_indicator', 'led-b', ?, ?, ?, 'outcome_unknown', 'now', 'now')`,
		commandID, "intent-"+commandID, key[:], []byte("{}"), make([]byte, 32)); err != nil {
		t.Fatalf("insert command: %v", err)
	}
	if err := f.service.BindCommand(t.Context(), domain.CommandBinding{CommandID: commandID, Target: "led-b", Device: device, Owner: ownerOne}); err != nil {
		t.Fatal(err)
	}
}

func TestResolutionWaitsOnlyForTheSameDeviceBootsCommands(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	deviceA := domain.DeviceBoot{DeviceID: "thermal-a", BootID: "boot-a"}
	deviceB := domain.DeviceBoot{DeviceID: "thermal-b", BootID: "boot-b"}
	stateA, _ := f.recordState(t, deviceA)
	f.recordState(t, deviceB)
	f.insertUnknownOutcome(t, "cmd-device-b", deviceB)
	if err := f.service.OpenReconciliation(t.Context(), deviceA, ownerOne, "device A receipt was not trustworthy"); err != nil {
		t.Fatal(err)
	}
	evidence := evidenceFor(t, stateA, "led-a")
	if cleared, err := f.service.ResolveReconciliation(t.Context(), deviceA, ownerOne, domain.ResolutionSucceeded, evidence); err != nil || !cleared {
		t.Fatalf("device A was blocked by device B: cleared=%v err=%v", cleared, err)
	}
	f.insertUnknownOutcome(t, "cmd-device-a", deviceA)
	if err := f.service.OpenReconciliation(t.Context(), deviceA, ownerOne, "again"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ResolveReconciliation(t.Context(), deviceA, ownerOne, domain.ResolutionSucceeded, evidence); err == nil || !strings.Contains(err.Error(), "still require dispatcher reconciliation") {
		t.Fatalf("unresolved command of device A = %v", err)
	}
}
