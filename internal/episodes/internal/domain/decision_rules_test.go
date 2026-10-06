package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

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
