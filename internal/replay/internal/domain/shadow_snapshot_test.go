package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
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
	if _, err := VerifiedSnapshot([]byte(`{"entity":{"id":"other"},"phase":"warning"}`), persisted); err == nil {
		t.Fatal("tampered snapshot accepted")
	}
	if _, err := VerifiedSnapshot([]byte(`not json`), persisted); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := VerifiedSnapshot(canonical, []byte(strings.Repeat("0", 32))); err == nil {
		t.Fatal("wrong digest accepted")
	}
}

func TestShadowEntityIDRequiresEntity(t *testing.T) {
	t.Parallel()
	if id, err := ShadowEntityID([]byte(`{"entity":{"id":"motor-1"}}`)); err != nil || id != "motor-1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	for name, snapshot := range map[string][]byte{
		"empty":     {},
		"no entity": []byte(`{"phase":"warning"}`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ShadowEntityID(snapshot); err == nil {
				t.Fatal("missing entity accepted")
			}
		})
	}
}
