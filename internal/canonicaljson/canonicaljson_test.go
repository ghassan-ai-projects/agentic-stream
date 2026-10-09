package canonicaljson_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// The facade exposes each domain rule unchanged: the same input gives the same
// canonical bytes, digests and verdicts through the public API.
func TestFacadeDelegatesEveryOperationToTheDomain(t *testing.T) {
	t.Parallel()
	document := map[string]any{"b": 1, "a": []any{"x", true}}
	raw, err := canonicaljson.Marshal(document)
	if err != nil || string(raw) != `{"a":["x",true],"b":1}` {
		t.Fatalf("marshal = %s err=%v", raw, err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, document)
	if err != nil || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest = %q err=%v", digest, err)
	}
	if !canonicaljson.Verify(canonicaljson.DomainSnapshot, document, digest) || canonicaljson.Verify(canonicaljson.DomainDecision, document, digest) {
		t.Fatal("verify did not bind the digest to its domain")
	}
	sum, err := canonicaljson.DecodeDigest(digest)
	if err != nil || canonicaljson.EncodeDigest(sum) != digest {
		t.Fatalf("decode/encode round trip failed: %v", err)
	}
	content := canonicaljson.ContentDigest(raw)
	stored, err := canonicaljson.DecodeDigest(content)
	if err != nil || canonicaljson.VerifyStored(raw, stored) != nil {
		t.Fatalf("stored verification failed: %v", err)
	}
	if err := canonicaljson.VerifyStored(bytes.ToUpper(raw), stored); err == nil {
		t.Fatal("tampered stored document accepted")
	}
}

// The raw-sum operations are the byte form of the string operations: they
// agree with Digest, Verify and the stored-bytes hash for the same input.
func TestFacadeRawSumOperationsAgreeWithTextForm(t *testing.T) {
	t.Parallel()
	document := map[string]any{"b": 1, "a": []any{"x", true}}
	digest, err := canonicaljson.Digest(canonicaljson.DomainCommand, document)
	if err != nil {
		t.Fatal(err)
	}
	canonical, sealed, err := canonicaljson.Seal(canonicaljson.DomainCommand, document)
	if err != nil || canonicaljson.EncodeDigest(sealed) != digest {
		t.Fatalf("Seal sum = %x want digest %s (err %v)", sealed, digest, err)
	}
	if want, _ := canonicaljson.Marshal(document); !bytes.Equal(canonical, want) {
		t.Fatalf("Seal json = %s, want %s", canonical, want)
	}
	sum, err := canonicaljson.DigestSum(canonicaljson.DomainCommand, document)
	if err != nil || !bytes.Equal(sum, sealed) || !canonicaljson.VerifySum(canonicaljson.DomainCommand, document, sum) {
		t.Fatalf("DigestSum/VerifySum disagree with Seal: %x vs %x (err %v)", sum, sealed, err)
	}
	if canonicaljson.VerifySum(canonicaljson.DomainDecision, document, sum) {
		t.Fatal("VerifySum accepted the wrong domain")
	}
	content := canonicaljson.Sum(canonical)
	if canonicaljson.EncodeDigest(content) != canonicaljson.ContentDigest(canonical) || !canonicaljson.HasSumLength(content) {
		t.Fatal("Sum does not agree with ContentDigest")
	}
}
