package app_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

type deviceControl struct {
	db        *storage.DB
	authority *deviceauthority.Service
}

func newDeviceControl(t *testing.T) deviceControl {
	t.Helper()
	db := storagetest.OpenTemp(t)
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute}
	return deviceControlOn(t, db, owner, deviceauthority.Config{})
}

func newVirtualClockDeviceControl(t *testing.T, lease time.Duration) (deviceControl, *sources.Virtual) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	clock := sources.NewVirtual(time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))
	owner := &runtimecontrol.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: lease, Now: clock.Now}
	return deviceControlOn(t, db, owner, deviceauthority.Config{ClaimLease: lease, Clock: clock}), clock
}

func deviceControlOn(t *testing.T, db *storage.DB, owner *runtimecontrol.RuntimeOwner, config deviceauthority.Config) deviceControl {
	t.Helper()
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim runtime owner: %v", err)
	}
	config.DB, config.Owner = db, owner
	config.Epochs = &runtimecontrol.EpochControl{DB: db}
	config.Outcomes = actions.CountUnresolvedOutcomes
	authority, err := deviceauthority.New(config)
	if err != nil {
		t.Fatalf("new device authority: %v", err)
	}
	return deviceControl{db: db, authority: authority}
}

func openThermalSession(t *testing.T, replies ...map[string]any) (*app.Session, *fakeDeviceTransport, *domain.CapabilityCatalog) {
	t.Helper()
	session, transport, catalog, _ := openThermalSessionWithControl(t, replies...)
	return session, transport, catalog
}

func openThermalSessionWithControl(t *testing.T, replies ...map[string]any) (*app.Session, *fakeDeviceTransport, *domain.CapabilityCatalog, deviceControl) {
	t.Helper()
	catalog := loadThermalCatalog(t)
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	transport.queue(mustDeviceFrames(t, replies...)...)
	control := newDeviceControl(t)
	return openSessionOn(t, transport, catalog, control.authority), transport, catalog, control
}

func reconciliationRequired(t *testing.T, control deviceControl) bool {
	t.Helper()
	required, err := control.authority.ReconciliationRequired(t.Context(), "thermal-01")
	if err != nil {
		t.Fatalf("read reconciliation barrier: %v", err)
	}
	return required
}

func storedReconciliationStatus(t *testing.T, control deviceControl) string {
	t.Helper()
	var status string
	if err := control.db.QueryRowContext(t.Context(), `SELECT status FROM device_reconciliation WHERE device_id = 'thermal-01'`).Scan(&status); err != nil {
		t.Fatalf("read stored reconciliation status: %v", err)
	}
	return status
}

func countAuthorityEvents(t *testing.T, control deviceControl, eventType string) int {
	t.Helper()
	var count int
	if err := control.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM device_authority_events WHERE event_type = ?`, eventType).Scan(&count); err != nil {
		t.Fatalf("count %s events: %v", eventType, err)
	}
	return count
}

func assertRestartedSessionRefusesOrdinaryCommands(t *testing.T, control deviceControl, catalog *domain.CapabilityCatalog, state map[string]any, want string) {
	t.Helper()
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, state)...)
	restarted := openSessionOn(t, transport, catalog, control.authority)
	bootID, _ := state["boot_id"].(string)
	_, sent, err := restarted.Exchange(t.Context(), materializedCommandWithBoot(t, catalog, "cmd-after-restart", idemKey(), bootID))
	assertRefusedBeforeSend(t, sent, err, want)
	if transport.sendCount() != 0 {
		t.Fatalf("restarted session sent %d frames across the barrier", transport.sendCount())
	}
}
