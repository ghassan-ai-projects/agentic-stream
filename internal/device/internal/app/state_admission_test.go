package app_test

import (
	"errors"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
)

func TestStateTransitionsRemainFencedAfterEpochKill(t *testing.T) {
	t.Parallel()
	t.Run("handshake", func(t *testing.T) {
		session, _, catalog, control := openThermalSessionWithControl(t)
		defer func() { _ = session.Close() }()
		epochs := &runtimecontrol.EpochControl{DB: control.db}
		if err := epochs.Kill(t.Context(), "epoch-1"); err != nil {
			t.Fatal(err)
		}
		state := goldenDeviceState()
		digest, _ := catalog.Digest()
		state["capability_digest"] = digest
		transport := &fakeDeviceTransport{frames: mustDeviceFrames(t, state)}
		_, err := app.OpenSession(t.Context(), app.SessionConfig{
			Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
			AllowedFirmwareDigests: []string{state["firmware_digest"].(string)}, OwnerEpoch: "epoch-1", Authority: control.authority,
		})
		if !errors.Is(err, runtimecontrol.ErrEpochKilled) || !transport.closed {
			t.Fatalf("handshake error=%v, closed=%v", err, transport.closed)
		}
	})
	t.Run("refresh", func(t *testing.T) {
		session, transport, catalog, control := openThermalSessionWithControl(t)
		defer func() { _ = session.Close() }()
		state := goldenDeviceState()
		digest, _ := catalog.Digest()
		state["capability_digest"] = digest
		transport.frames = append(transport.frames, mustDeviceFrames(t, state)...)
		if err := (&runtimecontrol.EpochControl{DB: control.db}).Kill(t.Context(), "epoch-1"); err != nil {
			t.Fatal(err)
		}
		if _, err := session.QueryState(t.Context()); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
			t.Fatalf("refresh error=%v", err)
		}
		if _, sent, err := session.Exchange(t.Context(), materializedCommand(t, catalog, "after-kill", idemKey())); err == nil || sent || transport.sendCount() != 0 {
			t.Fatalf("failed refresh allowed dispatch: sent=%v, error=%v", sent, err)
		}
	})
	t.Run("resolve", func(t *testing.T) {
		session, transport, catalog, control := openThermalSessionWithControl(t)
		defer func() { _ = session.Close() }()
		state := goldenDeviceState()
		digest, _ := catalog.Digest()
		state["capability_digest"], state["boot_id"] = digest, "boot-B"
		transport.frames = append(transport.frames, mustDeviceFrames(t, state)...)
		if _, err := session.QueryState(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := (&runtimecontrol.EpochControl{DB: control.db}).Kill(t.Context(), "epoch-1"); err != nil {
			t.Fatal(err)
		}
		cleared, err := session.ResolveReconciliation(t.Context(), "succeeded", reconciliationEvidence(t, state, "fan-01"))
		if cleared || !errors.Is(err, runtimecontrol.ErrEpochKilled) {
			t.Fatalf("resolution cleared=%v, error=%v", cleared, err)
		}
		required, err := control.authority.ReconciliationRequired(t.Context(), "thermal-01")
		if err != nil || !required {
			t.Fatalf("resolution lost durable reconciliation: required=%v, error=%v", required, err)
		}
	})
}
