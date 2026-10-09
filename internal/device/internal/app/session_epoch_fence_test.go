package app_test

import (
	"errors"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func killEpoch(t *testing.T, control deviceControl) {
	t.Helper()
	if err := (&runtimecontrol.EpochControl{DB: control.db}).Kill(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("kill epoch-1: %v", err)
	}
}

func TestKilledEpochRefusesTheDeviceHandshakeAndClosesTheLink(t *testing.T) {
	t.Parallel()
	control := newDeviceControl(t)
	catalog := loadThermalCatalog(t)
	killEpoch(t, control)
	transport := &fakeDeviceTransport{}
	transport.queue(mustDeviceFrames(t, stateFor(t, catalog))...)
	_, err := app.OpenSession(t.Context(), sessionConfig(t, transport, catalog, control.authority))
	if !errors.Is(err, runtimecontrol.ErrEpochKilled) || !transport.isClosed() {
		t.Fatalf("handshake error=%v, link closed=%v", err, transport.isClosed())
	}
}

func TestKilledEpochRefusesAStateRefreshAndTheCommandsAfterIt(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	queueState(t, transport, catalog)
	killEpoch(t, control)
	if _, err := session.QueryState(t.Context()); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("refresh error=%v, want ErrEpochKilled", err)
	}
	_, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "after-kill", idemKey()))
	assertRefusedBeforeSend(t, sent, err, "device session is not open")
	if transport.sendCount() != 0 {
		t.Fatalf("a killed epoch sent %d frames", transport.sendCount())
	}
}

func TestKilledEpochCannotResolveAReconciliationBarrier(t *testing.T) {
	t.Parallel()
	session, transport, catalog, control := openThermalSessionWithControl(t)
	state := queueState(t, transport, catalog, map[string]any{"boot_id": "boot-B"})
	if _, err := session.QueryState(t.Context()); err != nil {
		t.Fatalf("query state: %v", err)
	}
	killEpoch(t, control)
	cleared, err := session.ResolveReconciliation(t.Context(), "succeeded", reconciliationEvidence(t, state, "fan-01"))
	if cleared || !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("resolution cleared=%v, error=%v, want an ErrEpochKilled refusal", cleared, err)
	}
	if !reconciliationRequired(t, control) {
		t.Fatal("a refused resolution lost the durable reconciliation barrier")
	}
}
