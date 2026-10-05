package domain

import "fmt"

// Command is a materialized device command: one bounded operation on one
// target, bound to the boot the session expects.
type Command struct {
	ProtocolVersion int
	CommandID       string
	IdempotencyKey  string
	Target          string
	Operation       string
	Parameters      map[string]any
	ExpectedBootID  string
	NotBeforeMonoUS int64
	ExpiresAfterMs  int
	PolicyDigest    string
}

// safeStopOperation is the catalog operation that requests a device's safe
// state.
const safeStopOperation = "safe_stop"

// IsSafeStop reports whether the command requests the device's safe state.
func (c Command) IsSafeStop() bool {
	return c.Operation == safeStopOperation
}

// Document is the command's wire form. An empty idempotency key is omitted,
// which is the form a safe stop's identity is computed over before its key is
// set.
func (c Command) Document() map[string]any {
	document := map[string]any{
		"message_type": "command", "protocol_version": c.ProtocolVersion,
		"command_id": c.CommandID, "target": c.Target, "operation": c.Operation,
		"parameters": c.Parameters, "expected_boot_id": c.ExpectedBootID,
		"not_before_mono_us": c.NotBeforeMonoUS, "expires_after_ms": c.ExpiresAfterMs,
		"policy_digest": c.PolicyDigest,
	}
	if c.IdempotencyKey != "" {
		document["idempotency_key"] = c.IdempotencyKey
	}
	return document
}

// Identity is the command's digest without its command ID: two commands with
// the same identity are the same command, so a repeated idempotency key must
// carry the same identity.
func (c Command) Identity() (string, error) {
	identity := c.Document()
	delete(identity, "command_id")
	return documentDigest(identity)
}

// Output is a device's report of one output's current state.
type Output struct {
	Target    string
	Operation string
	Value     float64
	Energized bool
}

// State is a device's report of its identity, digests, safe state and current
// output. Document is the record as received; its digest binds reconciliation
// evidence.
type State struct {
	MessageType      string
	ProtocolVersion  int64
	DeviceID         string
	BootID           string
	FirmwareDigest   string
	CapabilityDigest string
	SafeState        bool
	Output           Output
	Document         map[string]any
}

// Digest is the sha256 reference of the state record as received.
func (s State) Digest() (string, error) {
	return StateDigest(s.Document)
}

// Receipt is a device's admission answer to a command. It is not proof of
// effect.
type Receipt struct {
	MessageType string
	CommandID   string
	BootID      string
	Accepted    bool
	RejectCode  *string
	Document    map[string]any
}

// Result is a device's terminal execution status for a command. It is not
// physical confirmation.
type Result struct {
	MessageType string
	CommandID   string
	BootID      string
	Status      string
	ErrorCode   *string
	Document    map[string]any
}

func sameOptional(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func documentDigest(document map[string]any) (string, error) {
	digest, err := commandDigest(document)
	if err != nil {
		return "", fmt.Errorf("digest device command identity: %w", err)
	}
	return digest, nil
}
