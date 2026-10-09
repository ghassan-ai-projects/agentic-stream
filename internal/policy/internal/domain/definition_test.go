package domain

import (
	"bytes"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestThePolicyDigestIsPinned(t *testing.T) {
	t.Parallel()
	digest, err := DigestForVersion("v1")
	if err != nil || digest != "sha256:473ca13620fa9323385b275037070a405ab5f1d9a3c911d1403d3475dc5fb75e" {
		t.Fatalf("policy digest = %s, %v; every stored evaluation and device command carries it", digest, err)
	}
}

func TestThePolicyDigestBindsTheDocumentAndItsVersion(t *testing.T) {
	t.Parallel()
	if _, err := DigestForVersion(""); err == nil {
		t.Fatal("a policy without a version has a digest")
	}
	digest, err := DigestForVersion("v1")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := canonicaljson.Digest(canonicaljson.DomainPolicy, CanonicalDocumentForVersion("v1"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := DigestForVersion("v2")
	if err != nil {
		t.Fatal(err)
	}
	if digest != expected || digest == other {
		t.Fatalf("digest %s, domain-separated document digest %s, v2 digest %s", digest, expected, other)
	}
}

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

func TestChangingAnySignedFieldChangesTheSigningBytes(t *testing.T) {
	t.Parallel()
	base := ApprovalAssertion{ApprovalID: "approval", IntentID: "intent", ApproverID: "human", RelayID: "relay", Approved: true}
	signed, err := ApprovalAssertionSigningBytes(base)
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*ApprovalAssertion){
		"the decision":   func(a *ApprovalAssertion) { a.Approved = false },
		"the relay":      func(a *ApprovalAssertion) { a.RelayID = "other" },
		"the approver":   func(a *ApprovalAssertion) { a.ApproverID = "other" },
		"the intent":     func(a *ApprovalAssertion) { a.IntentID = "other" },
		"the nonce":      func(a *ApprovalAssertion) { a.Nonce = "other" },
		"the expiry":     func(a *ApprovalAssertion) { a.ExpiresAt = a.ExpiresAt.Add(time.Second) },
		"the risk class": func(a *ApprovalAssertion) { a.RiskClass = "R3" },
	}
	for name, change := range changes {
		changed := base
		change(&changed)
		other, err := ApprovalAssertionSigningBytes(changed)
		if err != nil || bytes.Equal(signed, other) {
			t.Errorf("changing %s does not change the signing bytes (%v)", name, err)
		}
	}
	if !bytes.HasPrefix(signed, []byte(canonicaljson.DomainApproval)) {
		t.Error("signing bytes are not domain separated")
	}
}
