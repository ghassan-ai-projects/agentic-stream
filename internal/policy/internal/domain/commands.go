package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// NewCommand seals the command and its stable per-intent idempotency key.
func NewCommand(p CommandPreparation) (CommandRecord, error) {
	target := NormalizedTarget(p.Row.IntentID, p.Intent.Parameters)
	idempotency := canonicaljson.Sum([]byte(p.Row.TenantID + "|" + p.Row.IntentID + "|" + p.Row.IntentType + "|" + target))
	document := map[string]any{
		"command_id": p.ID, "intent_id": p.Row.IntentID, "tenant_id": p.Row.TenantID,
		"effector_route": p.Row.IntentType, "normalized_target": target,
		"idempotency_key": canonicaljson.EncodeDigest(idempotency),
		"status":          "prepared", "not_before_mono_us": 0, "policy_digest": p.PolicyDigest,
		"payload": p.Intent.Parameters, "created_at": kernel.FormatTime(p.Now),
	}
	return sealCommand(document, p.ID, target, idempotency)
}

func sealCommand(document map[string]any, commandID, target string, idempotency []byte) (CommandRecord, error) {
	commandJSON, commandSHA, err := canonicaljson.Seal(canonicaljson.DomainCommand, document)
	if err != nil {
		return CommandRecord{}, fmt.Errorf("seal command: %w", err)
	}
	return CommandRecord{ID: commandID, JSON: commandJSON, SHA: commandSHA, Key: idempotency, Target: target}, nil
}
