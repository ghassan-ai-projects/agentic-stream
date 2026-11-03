package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

type CommandBinding struct {
	CommandID        string
	Target           string
	NormalizedTarget string
	Device           DeviceBoot
	Owner            Owner
	CommandDigest    string
	NoReadback       bool
}

func (b CommandBinding) Complete() bool {
	return b.CommandID != "" && b.Target != "" && b.Device.Complete() && b.Owner.Complete()
}

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
	return existing.Target == requested.Target && existing.NormalizedTarget == requested.NormalizedTarget && existing.Device == requested.Device &&
		existing.Owner == requested.Owner && existing.NoReadback == requested.NoReadback && sameDigest(existing.CommandDigest, requested.CommandDigest)
}

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

func (b CommandBinding) commandTarget() string {
	if b.NormalizedTarget != "" {
		return b.NormalizedTarget
	}
	return b.Target
}

type CommandEvidence struct {
	CommandID string
	Target    string
	Evidence  map[string]any
}

func CheckCommandEvidence(binding *CommandBinding, presented CommandEvidence) error {
	if binding == nil {
		return nil
	}
	if binding.commandTarget() != presented.Target {
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
