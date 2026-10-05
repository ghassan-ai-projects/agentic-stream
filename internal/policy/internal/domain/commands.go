package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"time"
)

func NewCommand(commandID, policyDigest string, row IntentRecord, intent map[string]any, now time.Time) (CommandRecord, error) {
	target := NormalizedTarget(row.IntentID, intent)
	idempotency := sha256.Sum256([]byte(row.TenantID + "|" + row.IntentID + "|" + row.IntentType + "|" + target))
	document := map[string]any{
		"command_id": commandID, "intent_id": row.IntentID, "tenant_id": row.TenantID,
		"effector_route": row.IntentType, "normalized_target": target,
		"idempotency_key": "sha256:" + hex.EncodeToString(idempotency[:]),
		"status":          "prepared", "not_before_mono_us": 0, "policy_digest": policyDigest,
		"payload": intent["parameters"], "created_at": FormatTime(now),
	}
	return sealCommand(document, commandID, target, idempotency)
}

func sealCommand(document map[string]any, commandID, target string, idempotency [sha256.Size]byte) (CommandRecord, error) {
	commandJSON, err := canonicaljson.Marshal(document)
	if err != nil {
		return CommandRecord{}, fmt.Errorf("canonicalize command: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainCommand, document)
	if err != nil {
		return CommandRecord{}, fmt.Errorf("digest command: %w", err)
	}
	commandSHA, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return CommandRecord{}, fmt.Errorf("decode command digest: %w", err)
	}
	return CommandRecord{ID: commandID, JSON: commandJSON, SHA: commandSHA, Key: idempotency[:], Target: target}, nil
}
