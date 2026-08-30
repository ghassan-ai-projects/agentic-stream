package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func validateDeviceState(frame []byte, catalog *CapabilityCatalog, catalogDigest string, allowedCapabilityDigests, allowedFirmwareDigests []string) (map[string]any, error) {
	state, err := DecodeDeviceRecord(frame)
	if err != nil {
		return nil, err
	}
	if state["message_type"] != "state" {
		return nil, fmt.Errorf("device handshake returned %v, want state", state["message_type"])
	}
	if protocol := documentInt64(state, "protocol_version"); protocol != int64(catalog.ProtocolVersion) {
		return nil, fmt.Errorf("device protocol version %d is unsupported", protocol)
	}
	capabilityDigest := stateString(state, "capability_digest")
	if capabilityDigest != catalogDigest || !contains(allowedCapabilityDigests, capabilityDigest) {
		return nil, fmt.Errorf("device capability digest %q does not match the allow-listed catalog", capabilityDigest)
	}
	firmwareDigest := stateString(state, "firmware_digest")
	if !contains(allowedFirmwareDigests, firmwareDigest) {
		return nil, fmt.Errorf("device firmware digest %q is not allow-listed", firmwareDigest)
	}
	if stateString(state, "device_id") == "" || stateString(state, "boot_id") == "" {
		return nil, fmt.Errorf("device handshake identity is incomplete")
	}
	return state, nil
}

func stateString(state map[string]any, key string) string {
	value, _ := state[key].(string)
	return value
}

type cachedReceipt struct {
	commandDigest string
	receipt       map[string]any
}

type deviceExchangeError struct{ err error }

func (e *deviceExchangeError) Error() string { return e.err.Error() }
func (e *deviceExchangeError) Unwrap() error { return e.err }

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func cloneDocument(document map[string]any) map[string]any {
	clone := make(map[string]any, len(document))
	for key, value := range document {
		clone[key] = value
	}
	return clone
}

func semanticCommandDigest(command map[string]any) (string, error) {
	identity := cloneDocument(command)
	delete(identity, "command_id")
	digest, err := canonicaljson.Digest(canonicaljson.DomainCommand, identity)
	if err != nil {
		return "", fmt.Errorf("digest semantic device command: %w", err)
	}
	return digest, nil
}

func stateDigest(state map[string]any) (string, error) {
	data, err := canonicaljson.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("canonicalize device state: %w", err)
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}
