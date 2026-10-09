package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func validSnapshotDocument(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	document := map[string]any{
		"situation_id": "s1", "situation_version": 2, "situation_type": "motor_over_temp", "tenant_id": "tenant",
		"entity": map[string]any{"type": "motor", "id": "motor-1"}, "phase": "warning", "severity": 10.0,
		"completeness": "on_time", "event_horizon": "2026-01-01T00:00:00Z",
		"spec_digest": "sha256:" + strings.Repeat("2", 64), "facts": map[string]any{},
	}
	if mutate != nil {
		mutate(document)
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func persistedDigestOf(t *testing.T, raw []byte) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestValidateSnapshotEvidenceAcceptsBoundSnapshot(t *testing.T) {
	t.Parallel()
	raw := validSnapshotDocument(t, nil)
	evidence, err := ValidateSnapshotEvidence(raw, persistedDigestOf(t, raw), "tp", "ts", "s1", 2, "tenant")
	if err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	if evidence.EntityID != "motor-1" || evidence.Digest == "" || evidence.Document["tenant_id"] != "tenant" {
		t.Fatalf("evidence = %+v", evidence)
	}
}

func TestValidateSnapshotEvidenceRejectsTamperingAndIdentityDrift(t *testing.T) {
	t.Parallel()
	raw := validSnapshotDocument(t, nil)
	tampered := validSnapshotDocument(t, func(d map[string]any) { d["phase"] = "incident" })
	cases := []struct {
		name        string
		raw         []byte
		persisted   func() []byte
		situationID string
		version     int
		tenantID    string
		want        string
	}{
		{"digest mismatch", raw, func() []byte { return persistedDigestOf(t, tampered) }, "s1", 2, "tenant", "digest does not match"},
		{"situation drift", tampered, func() []byte { return persistedDigestOf(t, tampered) }, "other", 2, "tenant", "identity does not match"},
		{"version drift", tampered, func() []byte { return persistedDigestOf(t, tampered) }, "s1", 3, "tenant", "identity does not match"},
		{"tenant drift", tampered, func() []byte { return persistedDigestOf(t, tampered) }, "s1", 2, "other", "identity does not match"},
		{"invalid snapshot", []byte(`{}`), func() []byte { return nil }, "s1", 2, "tenant", "validate snapshot"},
		{"ambiguous bytes", contractstest.AmbiguousKeyJSON(raw, "phase"), func() []byte { return persistedDigestOf(t, raw) }, "s1", 2, "tenant", "unmarshal snapshot"},
		{"undecodable", []byte("nope"), func() []byte { return nil }, "s1", 2, "tenant", "unmarshal snapshot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidateSnapshotEvidence(tc.raw, tc.persisted(), "tp", "ts", tc.situationID, tc.version, tc.tenantID)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v want %q", err, tc.want)
			}
		})
	}
}

func TestSnapshotEntityIDRequiresADecodableEntityWithAnID(t *testing.T) {
	t.Parallel()
	if id, err := SnapshotEntityID([]byte(`{"entity":{"id":"motor-1"}}`)); err != nil || id != "motor-1" {
		t.Fatalf("id = %q err = %v", id, err)
	}
	if _, err := SnapshotEntityID([]byte(`{"entity":{}}`)); err == nil || !strings.Contains(err.Error(), "snapshot entity id is required") {
		t.Fatalf("missing entity error = %v", err)
	}
	if _, err := SnapshotEntityID([]byte(`{"entity":`)); err == nil || !strings.Contains(err.Error(), "decode snapshot entity") {
		t.Fatalf("undecodable snapshot error = %v", err)
	}
}
