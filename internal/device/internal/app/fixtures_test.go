package app_test

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
)

func loadThermalCatalog(t *testing.T) *domain.CapabilityCatalog {
	t.Helper()
	data, err := os.ReadFile("../../../contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json")
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	catalog, err := domain.LoadCapabilityCatalog(data)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func catalogDigest(t *testing.T, catalog *domain.CapabilityCatalog) string {
	t.Helper()
	digest, err := catalog.Digest()
	if err != nil {
		t.Fatalf("digest catalog: %v", err)
	}
	return digest
}

func idemKey() string { return "sha256:" + strings.Repeat("a", 64) }

func policyKey() string { return "sha256:" + strings.Repeat("b", 64) }

func firmwareDigest() string { return "sha256:" + strings.Repeat("c", 64) }

func goldenDeviceState() map[string]any {
	return map[string]any{
		"message_type":      "state",
		"protocol_version":  1,
		"device_id":         "thermal-01",
		"boot_id":           "boot-A",
		"firmware_digest":   firmwareDigest(),
		"capability_digest": "sha256:" + strings.Repeat("d", 64),
		"safe_state":        true,
	}
}

func stateFor(t *testing.T, catalog *domain.CapabilityCatalog, fields ...map[string]any) map[string]any {
	t.Helper()
	state := goldenDeviceState()
	state["capability_digest"] = catalogDigest(t, catalog)
	for _, extra := range fields {
		maps.Copy(state, extra)
	}
	return state
}

func queueState(t *testing.T, transport *fakeDeviceTransport, catalog *domain.CapabilityCatalog, fields ...map[string]any) map[string]any {
	t.Helper()
	state := stateFor(t, catalog, fields...)
	transport.queue(mustDeviceFrames(t, state)...)
	return state
}

func acceptedReceipt(commandID string) map[string]any {
	return map[string]any{"message_type": "receipt", "protocol_version": 1, "command_id": commandID, "boot_id": "boot-A", "accepted": true}
}

func rejectedReceipt(commandID, code string) map[string]any {
	receipt := acceptedReceipt(commandID)
	receipt["accepted"] = false
	receipt["reject_code"] = code
	return receipt
}

func terminalResult(receipt map[string]any) map[string]any {
	accepted, _ := receipt["accepted"].(bool)
	commandID, _ := receipt["command_id"].(string)
	status := "rejected"
	switch {
	case accepted && strings.HasPrefix(commandID, "safe-stop/"):
		status = "safe_state"
	case accepted:
		status = "executed"
	}
	result := map[string]any{
		"message_type": "result", "protocol_version": 1,
		"command_id": commandID, "boot_id": receipt["boot_id"], "status": status,
	}
	if status == "rejected" {
		result["error_code"] = receipt["reject_code"]
	}
	return result
}

func mustDeviceFrames(t *testing.T, documents ...map[string]any) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, len(documents)*2)
	for _, document := range documents {
		frames = append(frames, mustEncode(t, document))
		if document["message_type"] == "receipt" {
			frames = append(frames, mustEncode(t, terminalResult(document)))
		}
	}
	return frames
}

func mustEncode(t *testing.T, document map[string]any) []byte {
	t.Helper()
	frame, err := wire.Encode(document)
	if err != nil {
		t.Fatalf("encode device record: %v", err)
	}
	return frame
}

func indicatorCommand(commandID string) actionport.Command {
	return actionport.Command{
		CommandID: commandID, EffectorRoute: "set_indicator", NormalizedTarget: "led-01",
		IdempotencyKey: idemKey(), PolicyDigest: policyKey(), Payload: map[string]any{"state": "watch"},
	}
}

func materializedCommand(t *testing.T, catalog *domain.CapabilityCatalog, commandID, idempotency string) domain.Command {
	t.Helper()
	return materializedCommandWithBoot(t, catalog, commandID, idempotency, "boot-A")
}

func materializedCommandWithBoot(t *testing.T, catalog *domain.CapabilityCatalog, commandID, idempotency, bootID string) domain.Command {
	t.Helper()
	command := indicatorCommand(commandID)
	command.IdempotencyKey = idempotency
	materialized, err := catalog.Materialize(command, bootID)
	if err != nil {
		t.Fatalf("materialize %s: %v", commandID, err)
	}
	return materialized
}

func sessionConfig(t *testing.T, transport app.Transport, catalog *domain.CapabilityCatalog, authority *deviceauthority.Service) app.SessionConfig {
	t.Helper()
	return app.SessionConfig{
		Transport: transport, Catalog: catalog, AllowedCapabilityDigests: []string{catalogDigest(t, catalog)},
		AllowedFirmwareDigests: []string{firmwareDigest()}, OwnerEpoch: "epoch-1",
		OwnerInstance: "instance-1", Authority: authority,
	}
}

func openSessionOn(t *testing.T, transport app.Transport, catalog *domain.CapabilityCatalog, authority *deviceauthority.Service) *app.Session {
	t.Helper()
	session, err := app.OpenSession(t.Context(), sessionConfig(t, transport, catalog, authority))
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func assertRefusedBeforeSend(t *testing.T, sent bool, err error, want string) {
	t.Helper()
	if sent || err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("sent=%v, error=%v, want an unsent refusal containing %q", sent, err, want)
	}
}

func reconciliationEvidence(t *testing.T, state map[string]any, target string) map[string]any {
	t.Helper()
	feedback := map[string]any{"target": target, "observed_state": "safe", "observed_at": "2026-08-29T12:00:00Z"}
	evidence := map[string]any{
		"device_id":       state["device_id"],
		"boot_id":         state["boot_id"],
		"evidence_type":   "device_state_feedback",
		"target":          target,
		"state":           state,
		"state_digest":    canonicalDigest(t, state),
		"feedback":        feedback,
		"feedback_digest": canonicalDigest(t, feedback),
		"source":          "independent-feedback",
	}
	evidence["evidence_digest"] = canonicalDigest(t, evidence)
	return evidence
}

func canonicalDigest(t *testing.T, document map[string]any) string {
	t.Helper()
	data, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatalf("canonical marshal: %v", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
