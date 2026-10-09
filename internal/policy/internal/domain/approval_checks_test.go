package domain

import (
	"crypto/ed25519"
	"strings"
	"testing"
)

func assertRefusal(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Errorf("unexpected refusal: %v", err)
	case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
		t.Errorf("error = %v, want one mentioning %q", err, want)
	}
}

func TestRelayAndApproverMustBeDistinctNamedPrincipals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		approver, relay string
		want            string
	}{
		{"distinct principals", "a", "b", ""},
		{"nobody named", "", "", "distinct registered principals"},
		{"no relay", "a", "", "distinct registered principals"},
		{"no approver", "", "b", "distinct registered principals"},
		{"the approver relays their own decision", "a", "a", "distinct registered principals"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertRefusal(t, DistinctPrincipals(ApprovalResolution{Approver: test.approver, Relay: test.relay}), test.want)
		})
	}
}

func TestPrincipalReadsMustShowExactlyWhatApprovalNeeds(t *testing.T) {
	t.Parallel()
	public := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)).Public().(ed25519.PublicKey)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"one active relay", ActiveRelay(1, false), ""},
		{"no active relay", ActiveRelay(0, false), "relay principal is not active"},
		{"a relay read that failed", ActiveRelay(1, true), "relay principal is not active"},
		{"an ambiguous relay count", ActiveRelay(2, false), "relay principal is not active"},
		{"a valid approver key", ValidApproverKey(public, false), ""},
		{"no approver key", ValidApproverKey(nil, false), "no valid verification key"},
		{"a truncated approver key", ValidApproverKey(public[:16], false), "no valid verification key"},
		{"an approver key read that failed", ValidApproverKey(public, true), "no valid verification key"},
		{"approver authority granted", AuthorizedApprover(1, false), ""},
		{"approver authority granted twice", AuthorizedApprover(2, false), ""},
		{"no approver authority", AuthorizedApprover(0, false), "lacks authority"},
		{"an authority read that failed", AuthorizedApprover(1, true), "lacks authority"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertRefusal(t, test.err, test.want)
		})
	}
}

func TestAssertionSignatureBindsTheExactBytesAndKey(t *testing.T) {
	t.Parallel()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	public := key.Public().(ed25519.PublicKey)
	otherKey := ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef")).Public().(ed25519.PublicKey)
	assertion := []byte("signed")
	signature := ed25519.Sign(key, assertion)

	digest, err := VerifyAssertion(public, assertion, signature)
	if err != nil || len(digest) != 32 {
		t.Fatalf("a valid signature = %x, %v", digest, err)
	}
	for name, call := range map[string]func() ([]byte, error){
		"changed bytes": func() ([]byte, error) { return VerifyAssertion(public, []byte("changed"), signature) },
		"another key":   func() ([]byte, error) { return VerifyAssertion(otherKey, assertion, signature) },
		"no key":        func() ([]byte, error) { return VerifyAssertion(nil, assertion, nil) },
		"no signature":  func() ([]byte, error) { return VerifyAssertion(public, assertion, nil) },
	} {
		if digest, err := call(); err == nil || digest != nil {
			t.Errorf("%s: VerifyAssertion = %x, %v; want a refusal", name, digest, err)
		}
	}
}

func TestADigestMustBeComplete(t *testing.T) {
	t.Parallel()
	assertRefusal(t, CompleteDigest(make([]byte, 32), "assertion"), "")
	assertRefusal(t, CompleteDigest(nil, "assertion"), "assertion digest is incomplete")
	assertRefusal(t, CompleteDigest(make([]byte, 31), "intent"), "intent digest is incomplete")
}

func TestApprovalNonceBindsTheApprovalToItsIntent(t *testing.T) {
	t.Parallel()
	const pinned = "f5c26a8b40b74dfb1f32a590b0d2a186a7ac779cd71e17e8702052d59d2dff70"
	if got := ApprovalNonce("approval", "intent"); got != pinned {
		t.Fatalf("nonce = %s, want the pinned %s", got, pinned)
	}
	if ApprovalNonce("other", "intent") == pinned || ApprovalNonce("approval", "other") == pinned {
		t.Fatal("the nonce does not change with the approval or the intent")
	}
}
