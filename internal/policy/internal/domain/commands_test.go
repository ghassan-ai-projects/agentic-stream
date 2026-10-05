package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"testing"
	"time"
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
	if !bytes.Equal(command.Key, key[:]) || command.Target != "motor" || document["created_at"] != "2026-10-05T00:00:00Z" || document["policy_digest"] != "policy" || document["not_before_mono_us"] != float64(0) {
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
func TestApprovalNoticeRetainsEvidenceAndFallbacks(t *testing.T) {
	t.Parallel()
	row := IntentRecord{TenantID: "tenant", IntentID: "intent", DecisionID: "decision", SituationID: "situation", SituationVersion: 1, RiskClass: "R2", IntentSHA: make([]byte, 32)}
	intent := map[string]any{"evidence_ids": []any{"one", "", 1, "two"}, "parameters": nil}
	context := ApprovalContext{Snapshot: make([]byte, 32), Delta: map[string]any{"change": 1}, Decision: DecisionDocument{Summary: "  ", Hypothesis: "motor wear"}, Source: "source"}
	data := BuildApprovalNotification(ApprovalNotice{Row: row, ID: "approval", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), Intent: ProjectIntent(intent), Context: context})
	if data["summary"] != "Decision decision requires approval" || data["hypothesis"] != "motor wear" || data["source_authority"] != "source" {
		t.Fatal(data)
	}
	evidence := data["evidence"].([]string)
	if len(evidence) != 2 || evidence[0] != "one" || evidence[1] != "two" {
		t.Fatal(evidence)
	}
	if _, ok := intent["parameters"].(map[string]any); !ok {
		t.Fatal("parameters not filled")
	}
	raw, err := ApprovalRequestJSON(row, ProjectIntent(intent), ApprovalRequest{ID: "approval", Nonce: "nonce", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if document["nonce"] != "nonce" || document["notification"].(map[string]any)["hypothesis"] != "motor wear" {
		t.Fatal(document)
	}
}
