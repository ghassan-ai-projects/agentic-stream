package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func snapshotFixture(t *testing.T) ([]byte, []byte) {
	t.Helper()
	document := map[string]any{
		"situation_id": "s1", "situation_version": 1, "situation_type": "motor_over_temp",
		"tenant_id": "default", "entity": map[string]any{"type": "motor", "id": "motor-1"},
		"phase": "warning", "severity": 10.0, "completeness": "on_time",
		"event_horizon": "2026-01-01T00:00:00Z", "spec_digest": "sha256:" + strings.Repeat("2", 64),
		"facts": map[string]any{},
	}
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, document)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	return canonical, persisted
}

func TestVerifiedSnapshotAcceptsMatchingDigest(t *testing.T) {
	t.Parallel()
	canonical, persisted := snapshotFixture(t)
	verified, err := VerifiedSnapshot(canonical, persisted)
	if err != nil || string(verified) != string(canonical) {
		t.Fatalf("verified=%d err=%v", len(verified), err)
	}
}

func TestVerifiedSnapshotRejectsTampering(t *testing.T) {
	t.Parallel()
	canonical, persisted := snapshotFixture(t)
	for name, snapshot := range map[string][]byte{
		"tampered snapshot":         []byte(`{"entity":{"id":"other"},"phase":"warning"}`),
		"ambiguous snapshot bytes":  contractstest.AmbiguousKeyJSON(canonical, "phase"),
		"snapshot that is not JSON": []byte(`not json`),
	} {
		if _, err := VerifiedSnapshot(snapshot, persisted); err == nil || !strings.Contains(err.Error(), "verify shadow snapshot:") {
			t.Fatalf("%s = %v, want verify shadow snapshot failure", name, err)
		}
	}
	if _, err := VerifiedSnapshot(canonical, []byte(strings.Repeat("0", 32))); err == nil || !strings.Contains(err.Error(), "verify shadow snapshot:") {
		t.Fatalf("a wrong digest = %v, want verify shadow snapshot failure", err)
	}
}

func TestShadowEntityIDRequiresEntity(t *testing.T) {
	t.Parallel()
	if id, err := ShadowEntityID([]byte(`{"entity":{"id":"motor-1"}}`)); err != nil || id != "motor-1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	for name, tc := range map[string]struct {
		snapshot []byte
		wantErr  string
	}{
		"empty":     {[]byte{}, "decode shadow entity"},
		"no entity": {[]byte(`{"phase":"warning"}`), "entity id is required"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ShadowEntityID(tc.snapshot); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ShadowEntityID(%s) = %v, want %q", name, err, tc.wantErr)
			}
		})
	}
}
