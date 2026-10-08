package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// CommandBinding ties a command to the target, device boot and owner that
// delivered it, plus the command digest. It is written immediately before
// delivery and never changes.
type CommandBinding struct {
	CommandID     string
	Target        string
	Device        DeviceBoot
	Owner         Owner
	CommandDigest string
}

// Complete reports whether every identity of the binding is present. The
// digest is optional.
func (b CommandBinding) Complete() bool {
	return b.CommandID != "" && b.Target != "" && b.Device.Complete() && b.Owner.Complete()
}

// DecideBinding reports whether the command already carries this exact
// binding, which makes a repeat idempotent. A different binding is a conflict.
func DecideBinding(existing *CommandBinding, requested CommandBinding) (bool, error) {
	if existing == nil {
		return false, nil
	}
	if !sameBinding(*existing, requested) {
		return false, fmt.Errorf("command %q is already bound to a different device boot", requested.CommandID)
	}
	return true, nil
}

func sameBinding(existing, requested CommandBinding) bool {
	return existing.Target == requested.Target && existing.Device == requested.Device &&
		existing.Owner == requested.Owner && sameDigest(existing.CommandDigest, requested.CommandDigest)
}

// sameDigest compares digests by value; both must be canonical digests.
func sameDigest(existing, requested string) bool {
	if requested == "" || existing == "" {
		return requested == existing
	}
	want, err := canonicaljson.DecodeDigest(requested)
	if err != nil {
		return false
	}
	have, err := canonicaljson.DecodeDigest(existing)
	return err == nil && string(have) == string(want)
}

// CommandEvidence is reconciliation evidence presented for one command whose
// outcome is being reconciled.
type CommandEvidence struct {
	CommandID string
	Target    string
	Evidence  map[string]any
}

// CheckCommandEvidence requires reconciliation evidence for a device-bound
// command to name the bound target and the bound device boot. A command with
// no binding was not delivered to a device and needs no device evidence.
func CheckCommandEvidence(binding *CommandBinding, presented CommandEvidence) error {
	if binding == nil {
		return nil
	}
	if binding.Target != presented.Target {
		return fmt.Errorf("device command binding target does not match command %q", presented.CommandID)
	}
	if evidenceTarget, _ := presented.Evidence["target"].(string); evidenceTarget != binding.Target {
		return fmt.Errorf("device reconciliation evidence target does not match command %q", presented.CommandID)
	}
	if _, err := ParseReconciliationEvidence(presented.Evidence, binding.Device); err != nil {
		return fmt.Errorf("validate device reconciliation evidence: %w", err)
	}
	return nil
}
