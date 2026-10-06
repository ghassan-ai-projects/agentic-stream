package domain_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
)

func TestValidDecisionStorageUsesVerifiedDigest(t *testing.T) {
	t.Parallel()
	req := executorconformance.FixtureRequest()
	outcome, err := fixture.New().Execute(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	identity := episodeledger.Identity{EpisodeID: req.EpisodeID, AttemptID: req.AttemptID, Fence: req.Fence}
	input, err := domain.DecisionInput(req, identity, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	record := domain.ValidateDecision(outcome, input)
	if record.ValidationErr != nil {
		t.Fatal(record.ValidationErr)
	}
	if _, err := record.PrepareStorage(outcome.DecisionJSON); err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.DecodeDigest(outcome.DecisionSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "dec_"+req.EpisodeID || record.ValidationStatus() != "proposed" || !bytes.Equal(record.Digest, digest) || string(record.ValidationJSON) != "{}" {
		t.Fatalf("record = %#v", record)
	}
}

func TestInvalidDecisionStorageRetainsRawEvidence(t *testing.T) {
	t.Parallel()
	raw := []byte("not-json")
	record := domain.ValidateDecision(&domain.Outcome{DecisionJSON: raw}, decisions.Input{})
	if record.ValidationErr == nil || record.ID != "" {
		t.Fatal("invalid proposal accepted or invented identity")
	}
	if _, err := record.PrepareStorage(raw); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if record.ValidationStatus() != "rejected" || !bytes.Equal(record.Digest, hash[:]) || !bytes.Contains(record.ValidationJSON, []byte("raw_sha256")) {
		t.Fatalf("rejected record=%#v", record)
	}
	record.ValidationErr = &decisions.ValidationError{Reason: "stale_situation", Details: map[string]any{"message": "superseded"}}
	if reason := record.RejectionReason(); reason != "stale_situation" {
		t.Fatalf("typed rejection=%s", reason)
	}
	record.ValidationErr = errors.New("invalid input")
	if reason := record.RejectionReason(); reason != "schema_invalid" {
		t.Fatalf("fallback rejection=%s", reason)
	}
}
