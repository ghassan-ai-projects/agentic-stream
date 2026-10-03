package runartifact

import (
	"strings"
	"testing"
)

func TestManifestSpecFailurePrecedesPolicyFailure(t *testing.T) {
	err := verifyManifestBindings(Manifest{SpecDigest: "invalid-spec", PolicyDigest: "invalid-policy"}, []byte(`{}`), []byte(`broken`))
	if err == nil || !strings.Contains(err.Error(), "manifest spec digest") {
		t.Fatalf("expected spec binding failure before invalid policy, got %v", err)
	}
}

func TestChecksumDuplicateDoesNotReplaceFirstDigest(t *testing.T) {
	expected := map[string]string{"commands.jsonl": strings.Repeat("a", 64)}
	err := indexChecksumEntry(expected, strings.Repeat("b", 64)+"  commands.jsonl")
	if err == nil || !strings.Contains(err.Error(), "duplicate checksum entry") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
	if got := expected["commands.jsonl"]; got != strings.Repeat("a", 64) {
		t.Fatalf("duplicate changed original checksum to %s", got)
	}
}
