package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestCommandIdentityAndPayloadRemainBound(t *testing.T) {
	t.Parallel()
	row := IntentRecord{IntentID: "intent", TenantID: "tenant", IntentType: "ticket"}
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	intent := map[string]any{"parameters": map[string]any{"target": " motor ", "unknown": true}}
	command, err := NewCommand(CommandPreparation{ID: "command", PolicyDigest: "policy", Row: row, Intent: ProjectIntent(intent), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(command.JSON, &document); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte("tenant|intent|ticket|motor"))
	if !bytes.Equal(command.Key, key[:]) || command.Target != "motor" || document["created_at"] != "2026-10-05T00:00:00.000000000Z" || document["policy_digest"] != "policy" || document["not_before_mono_us"] != float64(0) {
		t.Fatal(document)
	}
	if !documentMatchesBytes(command.JSON, command.SHA, canonicaljson.DomainCommand) {
		t.Fatal("command seal changed")
	}
	if document["payload"].(map[string]any)["unknown"] != true {
		t.Fatal("payload lost")
	}
	changed, err := NewCommand(CommandPreparation{ID: "other", PolicyDigest: "policy", Row: row, Intent: ProjectIntent(intent), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(changed.Key, command.Key) || bytes.Equal(changed.SHA, command.SHA) {
		t.Fatal("idempotency binding changed")
	}
	intent["parameters"] = map[string]any{"bad": make(chan int)}
	if _, err := NewCommand(CommandPreparation{ID: "other", PolicyDigest: "policy", Row: row, Intent: ProjectIntent(intent), Now: now}); err == nil {
		t.Fatal("unencodable payload accepted")
	}
}

func TestCommandDigestPinsTheCreatedAtText(t *testing.T) {
	t.Parallel()
	row := IntentRecord{IntentID: "intent", TenantID: "tenant", IntentType: "ticket"}
	command, err := NewCommand(CommandPreparation{ID: "command", PolicyDigest: "policy", Row: row, Intent: ProjectIntent(map[string]any{"parameters": map[string]any{"target": "motor"}}), Now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(command.SHA); got != "8b49402ff606ea5b3798058293bc564562a109a2c6660c60d397210048c026dd" {
		t.Fatalf("command digest = %s: created_at is the nine-digit UTC text, so a change here moves every stored command_sha256 and idempotent replay", got)
	}
}

func documentMatchesBytes(raw, digest []byte, domain canonicaljson.Domain) bool {
	document, err := contractsv1.DecodeDocumentJSON(raw)
	return err == nil && contractsv1.VerifyDocumentDigest(domain, document, digest)
}
