package actions_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
)

type fakeDeviceTransport struct {
	mu         sync.Mutex
	frames     [][]byte
	sentFrames [][]byte
	sends      int
	receiveErr error
	sendErr    error
	closed     bool
}

func (t *fakeDeviceTransport) Send(_ context.Context, frame []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sendErr != nil {
		return t.sendErr
	}
	t.sends++
	t.sentFrames = append(t.sentFrames, append([]byte(nil), frame...))
	return nil
}

func (t *fakeDeviceTransport) Receive(_ context.Context) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.frames) > 0 {
		frame := t.frames[0]
		t.frames = t.frames[1:]
		return frame, nil
	}
	if t.receiveErr != nil {
		return nil, t.receiveErr
	}
	return nil, errors.New("fake transport has no queued frame")
}

func (t *fakeDeviceTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *fakeDeviceTransport) sendCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sends
}

func openThermalSession(t *testing.T, replies ...map[string]any) (*actions.DeviceSession, *fakeDeviceTransport, *actions.CapabilityCatalog) {
	t.Helper()
	catalog := loadThermalCatalog(t)
	catalogDigest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	state := goldenDeviceState()
	state["capability_digest"] = catalogDigest
	stateFrame, err := actions.EncodeDeviceRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	transport := &fakeDeviceTransport{frames: [][]byte{stateFrame}}
	for _, reply := range replies {
		frame, encodeErr := actions.EncodeDeviceRecord(reply)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		transport.frames = append(transport.frames, frame)
	}
	session, err := actions.OpenDeviceSession(context.Background(), actions.DeviceSessionConfig{
		Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{catalogDigest},
		AllowedFirmwareDigests: []string{goldenDeviceState()["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return session, transport, catalog
}

func materializedCommand(t *testing.T, catalog *actions.CapabilityCatalog, commandID, idempotency string) map[string]any {
	t.Helper()
	command, err := catalog.Materialize(actions.Command{
		CommandID: commandID, EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idempotency, PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	}, "boot-A")
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func acceptedReceipt(commandID string) map[string]any {
	return map[string]any{"message_type": "receipt", "protocol_version": 1, "command_id": commandID, "boot_id": "boot-A", "accepted": true}
}

func TestOpenDeviceSessionRequiresHandshakeAgreement(t *testing.T) {
	t.Parallel()
	catalog := loadThermalCatalog(t)
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"wrong capability", func(s map[string]any) { s["capability_digest"] = "sha256:" + strings.Repeat("e", 64) }},
		{"wrong firmware", func(s map[string]any) { s["firmware_digest"] = "sha256:" + strings.Repeat("f", 64) }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := goldenDeviceState()
			state["capability_digest"] = digest
			tc.mutate(state)
			badFrame, encodeErr := actions.EncodeDeviceRecord(state)
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			transport := &fakeDeviceTransport{frames: [][]byte{badFrame}}
			_, openErr := actions.OpenDeviceSession(context.Background(), actions.DeviceSessionConfig{
				Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{digest},
				AllowedFirmwareDigests: []string{goldenDeviceState()["firmware_digest"].(string)}, AuthorityEpoch: "epoch-1",
			})
			if openErr == nil {
				t.Fatal("handshake mismatch opened a session")
			}
			if transport.sendCount() != 0 {
				t.Fatal("handshake failure sent a command")
			}
		})
	}
	t.Run("wrong protocol", func(t *testing.T) {
		state := goldenDeviceState()
		state["capability_digest"] = digest
		frame, encodeErr := actions.EncodeDeviceRecord(state)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		frame = bytes.Replace(frame, []byte("\"protocol_version\":1"), []byte("\"protocol_version\":2"), 1)
		transport := &fakeDeviceTransport{frames: [][]byte{frame}}
		if _, openErr := actions.OpenDeviceSession(context.Background(), actions.DeviceSessionConfig{
			Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{digest}, AuthorityEpoch: "epoch-1",
		}); openErr == nil {
			t.Fatal("unsupported protocol opened a session")
		}
	})
}

func TestDeviceSessionCachesOnlyMatchingIdempotentCommands(t *testing.T) {
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	defer func() { _ = session.Close() }()
	idempotency := idemKey()
	command := materializedCommand(t, catalog, "cmd-1", idempotency)
	first, sent, err := session.Exchange(context.Background(), command)
	if err != nil || !sent || first["command_id"] != "cmd-1" {
		t.Fatalf("first exchange receipt=%v sent=%v err=%v", first, sent, err)
	}
	duplicate := materializedCommand(t, catalog, "cmd-2", idempotency)
	second, sent, err := session.Exchange(context.Background(), duplicate)
	if err != nil || !sent || second["command_id"] != "cmd-1" {
		t.Fatalf("duplicate exchange receipt=%v sent=%v err=%v", second, sent, err)
	}
	conflicting := materializedCommand(t, catalog, "cmd-3", idemKey())
	conflicting["parameters"] = map[string]any{"brightness_permille": 1, "pattern": "off"}
	conflicting["idempotency_key"] = idempotency
	if _, sent, err := session.Exchange(context.Background(), conflicting); err == nil || sent {
		t.Fatalf("conflicting idempotency exchange sent=%v err=%v", sent, err)
	}
	if got := transport.sendCount(); got != 1 {
		t.Fatalf("transport sends=%d, want 1", got)
	}
}

func TestDeviceSessionTreatsPostSendFailureAsAmbiguous(t *testing.T) {
	session, transport, catalog := openThermalSession(t)
	defer func() { _ = session.Close() }()
	transport.receiveErr = errors.New("gateway receive timeout")
	_, sent, err := session.Exchange(context.Background(), materializedCommand(t, catalog, "cmd-1", idemKey()))
	if err == nil || !sent {
		t.Fatalf("post-send failure sent=%v err=%v", sent, err)
	}
}

func TestDeviceSessionTreatsSendFailureAsPreSend(t *testing.T) {
	session, transport, catalog := openThermalSession(t)
	defer func() { _ = session.Close() }()
	transport.sendErr = errors.New("gateway refused write")
	_, sent, err := session.Exchange(context.Background(), materializedCommand(t, catalog, "cmd-1", idemKey()))
	if err == nil || sent {
		t.Fatalf("send failure sent=%v err=%v", sent, err)
	}
	if transport.sendCount() != 0 {
		t.Fatalf("transport sends=%d, want 0", transport.sendCount())
	}
}

func TestDeviceSessionInvalidatesAfterFailedRefresh(t *testing.T) {
	session, transport, catalog := openThermalSession(t)
	defer func() { _ = session.Close() }()
	state := goldenDeviceState()
	state["capability_digest"] = "sha256:" + strings.Repeat("e", 64)
	frame, err := actions.EncodeDeviceRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, frame)
	if err := session.RefreshState(context.Background()); err == nil {
		t.Fatal("invalid refresh unexpectedly succeeded")
	}
	if _, sent, err := session.Exchange(context.Background(), materializedCommand(t, catalog, "cmd-1", idemKey())); err == nil || sent {
		t.Fatalf("invalidated session exchanged command sent=%v err=%v", sent, err)
	}
}

func TestDeviceSessionRefreshFencesBootAndReceipts(t *testing.T) {
	session, transport, catalog := openThermalSession(t, acceptedReceipt("cmd-1"))
	defer func() { _ = session.Close() }()
	command := materializedCommand(t, catalog, "cmd-1", idemKey())
	if _, _, err := session.Exchange(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	state := goldenDeviceState()
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatal(err)
	}
	state["capability_digest"] = digest
	state["boot_id"] = "boot-B"
	frame, err := actions.EncodeDeviceRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	transport.frames = append(transport.frames, frame)
	if err := session.RefreshState(context.Background()); err != nil {
		t.Fatal(err)
	}
	if session.BootID() != "boot-B" {
		t.Fatalf("boot id=%q, want boot-B", session.BootID())
	}
	if _, sent, err := session.Exchange(context.Background(), command); err == nil || sent {
		t.Fatalf("old-boot command sent=%v err=%v", sent, err)
	}
}
