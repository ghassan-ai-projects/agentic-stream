package domain

import (
	"fmt"
	"maps"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// CommandIdentity is a device command's digest without its command ID: two
// commands with the same identity are the same command, so a repeated
// idempotency key must carry the same identity.
func CommandIdentity(command map[string]any) (string, error) {
	identity := maps.Clone(command)
	delete(identity, "command_id")
	digest, err := canonicaljson.Digest(canonicaljson.DomainCommand, identity)
	if err != nil {
		return "", fmt.Errorf("digest device command identity: %w", err)
	}
	return digest, nil
}

// StateDigest is the sha256 reference of a device state's canonical JSON.
// Reconciliation evidence binds the latest state by this digest.
func StateDigest(state map[string]any) (string, error) {
	data, err := canonicaljson.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("canonicalize device state: %w", err)
	}
	return canonicaljson.ContentDigest(data), nil
}

// StateAllowlist is what a device state must match before the session trusts
// it: the catalog's protocol version and digest, and an allowed firmware.
type StateAllowlist struct {
	ProtocolVersion  int
	CatalogDigest    string
	FirmwareDigests  []string
	CapabilityDigest []string
}

// CheckState requires a state record whose protocol, capability and firmware
// digests are allowed and whose device and boot identity are complete.
func CheckState(state map[string]any, allowed StateAllowlist) error {
	if state["message_type"] != "state" {
		return fmt.Errorf("device handshake returned %v, want state", state["message_type"])
	}
	if protocol := documentInt64(state, "protocol_version"); protocol != int64(allowed.ProtocolVersion) {
		return fmt.Errorf("device protocol version %d is unsupported", protocol)
	}
	return checkStateIdentity(state, allowed)
}

func checkStateIdentity(state map[string]any, allowed StateAllowlist) error {
	capabilityDigest := documentString(state, "capability_digest")
	if capabilityDigest != allowed.CatalogDigest || !slices.Contains(allowed.CapabilityDigest, capabilityDigest) {
		return fmt.Errorf("device capability digest %q does not match the allow-listed catalog", capabilityDigest)
	}
	if firmwareDigest := documentString(state, "firmware_digest"); !slices.Contains(allowed.FirmwareDigests, firmwareDigest) {
		return fmt.Errorf("device firmware digest %q is not allow-listed", firmwareDigest)
	}
	if documentString(state, "device_id") == "" || documentString(state, "boot_id") == "" {
		return fmt.Errorf("device handshake identity is incomplete")
	}
	return nil
}

// ReceiptMatches reports whether a receipt answers the command on the current
// boot.
func ReceiptMatches(receipt, command map[string]any, bootID string) bool {
	return receipt["message_type"] == "receipt" &&
		receipt["command_id"] == command["command_id"] &&
		receipt["boot_id"] == bootID
}

// ResultMatches reports whether a result answers the command on the current
// boot and agrees with its receipt: an accepted command executed (or, for a
// safe stop, reached the safe state) without an error code, and a rejected
// command reports the receipt's reject code.
func ResultMatches(result, command map[string]any, bootID string, receipt map[string]any) bool {
	if result["message_type"] != "result" || result["command_id"] != command["command_id"] || result["boot_id"] != bootID {
		return false
	}
	status, _ := result["status"].(string)
	if status == "" {
		return false
	}
	return resultAgreesWithReceipt(result, command, receipt, status)
}

func resultAgreesWithReceipt(result, command, receipt map[string]any, status string) bool {
	accepted, _ := receipt["accepted"].(bool)
	if !accepted {
		return status == "rejected" && result["error_code"] == receipt["reject_code"]
	}
	if documentString(command, "operation") == "safe_stop" {
		return status == "safe_state" && result["error_code"] == nil
	}
	return status == "executed" && result["error_code"] == nil
}

func documentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func documentInt64(document map[string]any, key string) int64 {
	value, _ := document[key].(float64)
	return int64(value)
}
