package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func TestNewBootOpensABarrierClearedOnlyByEvidenceBoundToTheLatestState(t *testing.T) {
	t.Parallel()
	control := newDeviceControl(t)
	catalog := loadThermalCatalog(t)
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	session := openSessionOn(t, transport, catalog, control.authority)
	bootB := queueState(t, transport, catalog, map[string]any{"boot_id": "boot-B"})
	if _, err := session.QueryState(t.Context()); err != nil {
		t.Fatalf("query state: %v", err)
	}
	if transport.stateQueryCount() != 1 {
		t.Fatalf("state queries=%d, want 1", transport.stateQueryCount())
	}
	command := materializedCommandWithBoot(t, catalog, "cmd-barrier", idemKey(), "boot-B")
	_, sent, err := session.Exchange(t.Context(), command)
	if sent || !errors.Is(err, authority.ErrReconciliationRequired) {
		t.Fatalf("barrier allowed an ordinary command: sent=%v err=%v", sent, err)
	}
	cleared, err := session.ResolveReconciliation(t.Context(), "succeeded", reconciliationEvidence(t, bootB, "fan-01"))
	if err != nil || !cleared {
		t.Fatalf("resolve barrier cleared=%v err=%v", cleared, err)
	}
	receipt := acceptedReceipt("cmd-barrier")
	receipt["boot_id"] = "boot-B"
	transport.queue(mustDeviceFrames(t, receipt)...)
	if _, sent, err := session.Exchange(t.Context(), command); err != nil || !sent {
		t.Fatalf("cleared barrier did not allow the command: sent=%v err=%v", sent, err)
	}
	if status := storedReconciliationStatus(t, control); status != "clear" {
		t.Fatalf("stored barrier status=%q, want clear", status)
	}
}

func TestSafeStopOutranksTheBarrierAndRefusesLaterOrdinaryWorkEvenAfterRestart(t *testing.T) {
	t.Parallel()
	control := newDeviceControl(t)
	catalog := loadThermalCatalog(t)
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	session := openSessionOn(t, transport, catalog, control.authority)
	bootB := queueState(t, transport, catalog, map[string]any{"boot_id": "boot-B"})
	safeStopReceipt := acceptedReceipt("safe-stop/fan-01")
	safeStopReceipt["boot_id"] = "boot-B"
	transport.queue(mustDeviceFrames(t, safeStopReceipt)...)
	if _, err := session.QueryState(t.Context()); err != nil {
		t.Fatalf("query state: %v", err)
	}
	if _, err := app.NewGatewayEffector(session, catalog).SafeStop(t.Context(), "fan-01"); err != nil {
		t.Fatalf("safe stop behind the barrier failed: %v", err)
	}
	_, sent, err := session.Exchange(t.Context(), materializedCommandWithBoot(t, catalog, "cmd-after-stop", idemKey(), "boot-B"))
	assertRefusedBeforeSend(t, sent, err, "safe stop has priority")
	if transport.sendCount() != 1 {
		t.Fatalf("transport sends=%d, want only the safe stop", transport.sendCount())
	}
	if completed := countAuthorityEvents(t, control, "safe_stop_completed"); completed != 1 {
		t.Fatalf("durable safe-stop completion events=%d, want 1", completed)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	assertRestartedSessionRefusesOrdinaryCommands(t, control, catalog, bootB, "safe stop has priority")
}

func TestLosingTheOwnerLeaseAfterSendingMakesTheOutcomeUnknownAndPersistsTheBarrier(t *testing.T) {
	t.Parallel()
	control, clock := newVirtualClockDeviceControl(t, 10*time.Second)
	catalog := loadThermalCatalog(t)
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	session := openSessionOn(t, transport, catalog, control.authority)
	transport.queue(mustDeviceFrames(t, acceptedReceipt("cmd-unknown"))...)
	transport.receiveHook = func() { clock.Advance(11 * time.Second) }
	_, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-unknown", idemKey()))
	if !sent || !errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) || !strings.Contains(err.Error(), "authority lost during device exchange") {
		t.Fatalf("lease lost after the transport: sent=%v err=%v, want a sent outcome caused by ErrRuntimeOwnerBusy", sent, err)
	}
	if status := storedReconciliationStatus(t, control); status != "required" {
		t.Fatalf("lease loss persisted barrier status %q, want required", status)
	}
}

func TestSessionStopsExchangingWhenTheReconciliationStoreFails(t *testing.T) {
	t.Parallel()
	control := newDeviceControl(t)
	catalog := loadThermalCatalog(t)
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	session := openSessionOn(t, transport, catalog, control.authority)
	if _, err := control.db.ExecContext(t.Context(), "DROP TABLE device_reconciliation"); err != nil {
		t.Fatalf("drop reconciliation table: %v", err)
	}
	queueState(t, transport, catalog)
	if _, err := session.QueryState(t.Context()); err == nil {
		t.Fatal("refresh succeeded after the reconciliation store failed")
	}
	_, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "cmd-after-store-failure", idemKey()))
	assertRefusedBeforeSend(t, sent, err, "device session is not open")
	if transport.sendCount() != 0 {
		t.Fatalf("transport sends=%d, want 0", transport.sendCount())
	}
}

func TestStartupBarrierCannotBeResolvedWithoutAFreshStateQuery(t *testing.T) {
	t.Parallel()
	control := newDeviceControl(t)
	catalog := loadThermalCatalog(t)
	first := &fakeDeviceTransport{}
	first.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	session1 := openSessionOn(t, first, catalog, control.authority)
	queueState(t, first, catalog, map[string]any{"boot_id": "boot-B"})
	if _, err := session1.QueryState(t.Context()); err != nil {
		t.Fatalf("query state on the first session: %v", err)
	}
	if err := session1.Close(); err != nil {
		t.Fatalf("close first session: %v", err)
	}

	stateB := stateFor(t, catalog, map[string]any{"boot_id": "boot-B"})
	second := &fakeDeviceTransport{}
	second.queue(mustDeviceFrames(t, stateB, stateB)...)
	session2 := openSessionOn(t, second, catalog, control.authority)
	evidence := reconciliationEvidence(t, stateB, "fan-01")
	if cleared, err := session2.ResolveReconciliation(t.Context(), "succeeded", evidence); cleared || err == nil {
		t.Fatalf("startup barrier resolved without a fresh state query: cleared=%v err=%v", cleared, err)
	}
	if _, err := session2.QueryState(t.Context()); err != nil {
		t.Fatalf("fresh state query: %v", err)
	}
	if cleared, err := session2.ResolveReconciliation(t.Context(), "succeeded", evidence); err != nil || !cleared {
		t.Fatalf("fresh state query did not permit reconciliation: cleared=%v err=%v", cleared, err)
	}
}
