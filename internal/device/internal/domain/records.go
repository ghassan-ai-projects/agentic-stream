package domain

import (
	"fmt"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func commandDigest(document map[string]any) (string, error) {
	return canonicaljson.Digest(canonicaljson.DomainCommand, document) //nolint:wrapcheck // documentDigest names the command identity.
}

// StateDigest is the sha256 reference of a device state's canonical JSON.
// Reconciliation evidence binds the latest state by this digest.
func StateDigest(document map[string]any) (string, error) {
	data, err := canonicaljson.Marshal(document)
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
func CheckState(state State, allowed StateAllowlist) error {
	if state.MessageType != "state" {
		return fmt.Errorf("device handshake returned %v, want state", state.Document["message_type"])
	}
	if state.ProtocolVersion != int64(allowed.ProtocolVersion) {
		return fmt.Errorf("device protocol version %d is unsupported", state.ProtocolVersion)
	}
	return checkStateIdentity(state, allowed)
}

func checkStateIdentity(state State, allowed StateAllowlist) error {
	if state.CapabilityDigest != allowed.CatalogDigest || !slices.Contains(allowed.CapabilityDigest, state.CapabilityDigest) {
		return fmt.Errorf("device capability digest %q does not match the allow-listed catalog", state.CapabilityDigest)
	}
	if !slices.Contains(allowed.FirmwareDigests, state.FirmwareDigest) {
		return fmt.Errorf("device firmware digest %q is not allow-listed", state.FirmwareDigest)
	}
	if state.DeviceID == "" || state.BootID == "" {
		return fmt.Errorf("device handshake identity is incomplete")
	}
	return nil
}

// ReceiptMatches reports whether a receipt answers the command on the current
// boot.
func ReceiptMatches(receipt Receipt, command Command, bootID string) bool {
	return receipt.MessageType == "receipt" && receipt.CommandID == command.CommandID && receipt.BootID == bootID
}

// ResultMatches reports whether a result answers the command on the current
// boot and agrees with its receipt: an accepted command executed (or, for a
// safe stop, reached the safe state) without an error code, and a rejected
// command reports the receipt's reject code.
func ResultMatches(result Result, command Command, bootID string, receipt Receipt) bool {
	if result.MessageType != "result" || result.CommandID != command.CommandID || result.BootID != bootID || result.Status == "" {
		return false
	}
	if !receipt.Accepted {
		return result.Status == "rejected" && sameOptional(result.ErrorCode, receipt.RejectCode)
	}
	if command.IsSafeStop() {
		return result.Status == "safe_state" && result.ErrorCode == nil
	}
	return result.Status == "executed" && result.ErrorCode == nil
}
