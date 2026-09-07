package runartifact

import (
	"encoding/base64"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestVerifySituationRowUsesSnapshotDigestDomain(t *testing.T) {
	snapshot := map[string]any{
		"situation_id":      "sit-1",
		"situation_version": 1,
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	digestBytes, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}

	row := map[string]any{
		"snapshot_json":   snapshot,
		"snapshot_sha256": base64.StdEncoding.EncodeToString(digestBytes),
	}
	if err := verifyLedgerRow("situations.jsonl", row); err != nil {
		t.Fatalf("verify situation snapshot row: %v", err)
	}
}
