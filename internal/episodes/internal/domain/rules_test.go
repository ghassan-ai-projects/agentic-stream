package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func requestWithBudget(wallTime string) []byte {
	document := map[string]any{"budget": map[string]any{"wall_time": wallTime}}
	raw, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestParseWallTimeBudgetValidatesBounds(t *testing.T) {
	t.Parallel()
	if duration, err := ParseWallTimeBudget(requestWithBudget("90s")); err != nil || duration != 90*time.Second {
		t.Fatalf("budget = %v err = %v", duration, err)
	}
	if duration, err := ParseWallTimeBudget(requestWithBudget("")); err != nil || duration != 0 {
		t.Fatalf("empty budget = %v err = %v", duration, err)
	}
	for name, wallTime := range map[string]string{"zero": "0s", "negative": "-5s", "unparseable": "soon"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseWallTimeBudget(requestWithBudget(wallTime)); err == nil {
				t.Fatal("invalid budget accepted")
			}
		})
	}
	if _, err := ParseWallTimeBudget([]byte("not-json")); err == nil || !strings.Contains(err.Error(), "decode episode budget") {
		t.Fatalf("err = %v", err)
	}
}

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

func TestSnapshotEntityIDRequiresEntity(t *testing.T) {
	t.Parallel()
	if id, err := SnapshotEntityID([]byte(`{"entity":{"id":"motor-1"}}`)); err != nil || id != "motor-1" {
		t.Fatalf("id = %q err = %v", id, err)
	}
	if _, err := SnapshotEntityID([]byte(`{"entity":{}}`)); err == nil {
		t.Fatal("missing entity accepted")
	}
}

func TestExecutionFailureClassification(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err    error
		reason string
		status episodeledger.AttemptStatus
	}{
		{&BudgetExceededError{Metric: "wall_time"}, "budget_exhausted", episodeledger.AttemptFailed},
		{BudgetTelemetryMissingError{}, "budget_telemetry_missing", episodeledger.AttemptFailed},
		{context.Canceled, "worker_cancelled", episodeledger.AttemptCancelled}, //nolint:misspell // Durable protocol reason is frozen as cancelled.
		{context.DeadlineExceeded, "worker_deadline_exceeded", episodeledger.AttemptTimedOut},
		{errors.New("boom"), "worker_execution_failed", episodeledger.AttemptFailed},
		{fmt.Errorf("wrap: %w", context.Canceled), "worker_cancelled", episodeledger.AttemptCancelled}, //nolint:misspell // Durable protocol reason is frozen as cancelled.
	}
	for _, tc := range cases {
		if got := ExecutionFailureReason(tc.err); got != tc.reason {
			t.Fatalf("reason(%v) = %q want %q", tc.err, got, tc.reason)
		}
		if got := ExecutionFailureStatus(tc.err); got != tc.status {
			t.Fatalf("status(%v) = %v want %v", tc.err, got, tc.status)
		}
	}
}

func TestStorageDecisionDigestPrefersContractDigest(t *testing.T) {
	t.Parallel()
	decision := map[string]any{"decision_id": "d1", "confidence": 0.5}
	raw, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	digest, hasContract := StorageDecisionDigest(raw)
	if !hasContract || len(digest) != 32 {
		t.Fatalf("digest = %x hasContract = %v", digest, hasContract)
	}
	noDigest, hasContract := StorageDecisionDigest([]byte("not-json"))
	if hasContract || len(noDigest) != 32 {
		t.Fatalf("raw-hash fallback = %x hasContract = %v", noDigest, hasContract)
	}
	if id := DecisionIDFromJSON(raw); id != "d1" {
		t.Fatalf("decision id = %q", id)
	}
	if id := DecisionIDFromJSON([]byte("nope")); id != "" {
		t.Fatalf("undecodable decision id = %q", id)
	}
}

func TestValidationFailureJSONRecordsReasonAndRawHash(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"decision_id":"d1"}`)
	document, err := ValidationFailureJSON(errors.New("bad shape"), raw, false)
	if err != nil {
		t.Fatal(err)
	}
	var details map[string]any
	if err := json.Unmarshal(document, &details); err != nil {
		t.Fatal(err)
	}
	if details["reason"] != "schema_invalid" || details["raw_sha256"] == nil {
		t.Fatalf("failure document = %s", document)
	}
	withDigest, err := ValidationFailureJSON(errors.New("bad shape"), raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withDigest), "raw_sha256") {
		t.Fatalf("contract-digest document leaks raw hash: %s", withDigest)
	}
}
