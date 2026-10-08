package domain

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestApprovalAssertionSigningBytesPinTheExpiryText(t *testing.T) {
	t.Parallel()
	assertion := ApprovalAssertion{
		Approved: true, ApprovalID: "approval", IntentID: "intent", DecisionID: "decision", TenantID: "tenant", SituationID: "situation",
		SituationVersion: 2, RiskClass: "R2", IntentDigest: "i", DecisionDigest: "d",
		ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), Nonce: "nonce", ApproverID: "human", RelayID: "relay",
	}
	signed, err := ApprovalAssertionSigningBytes(assertion)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"approval_id":"approval","approved":true,"approver_id":"human","decision_digest":"d","decision_id":"decision","expires_at":"2099-01-01T00:00:00.000000000Z","intent_digest":"i","intent_id":"intent","nonce":"nonce","relay_id":"relay","risk_class":"R2","situation_id":"situation","situation_version":2,"tenant_id":"tenant"}`
	if got := string(signed); got != string(canonicaljson.DomainApproval)+want {
		t.Fatalf("signing bytes changed: every approval relay signs these exact bytes\n got %s\nwant %s", got, string(canonicaljson.DomainApproval)+want)
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
