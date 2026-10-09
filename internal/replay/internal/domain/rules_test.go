package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestShadowManifestAdmissionPrecedesDecisionAndSnapshot(t *testing.T) {
	t.Parallel()
	_, err := ShadowRules{}.ValidateOutput(ShadowInput{SnapshotJSON: []byte(`broken`)}, ShadowOutput{ExecutorVersion: "test", ManifestSHA256: "invalid", DecisionJSON: []byte(`broken`)}, time.Time{})
	if err == nil || !strings.HasPrefix(err.Error(), "manifest digest:") {
		t.Fatalf("expected manifest rejection first, got %v", err)
	}
}

func TestRecordedAttemptIdentityPrecedesFence(t *testing.T) {
	t.Parallel()
	entry := RecordedEntry{EpisodeKey: "episode", EpisodeID: "ep", AttemptID: "attempt", Fence: 2}
	err := ValidateRecordedAttempt(entry, map[string]any{"episode_id": "ep", "attempt_id": "wrong", "fence": float64(3)})
	if err == nil || !strings.Contains(err.Error(), "mismatched attempt identity") {
		t.Fatalf("expected attempt identity rejection first, got %v", err)
	}
}

func TestEpochFromEarliestDefaultsToUnixOrigin(t *testing.T) {
	t.Parallel()
	epoch := EpochFromEarliest(time.Time{})
	if !epoch.Equal(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("zero epoch = %v", epoch)
	}
	earliest := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if got := EpochFromEarliest(earliest); !got.Equal(earliest) {
		t.Fatalf("earliest epoch = %v", got)
	}
}

func TestEnvelopeProcessingTimePrefersIngestedAt(t *testing.T) {
	t.Parallel()
	ingested := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	event := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, ok := EnvelopeProcessingTime(contractsv1.Envelope{IngestedAt: ingested, EventTime: event}); !ok || !got.Equal(ingested) {
		t.Fatalf("processing time = %v ok=%v", got, ok)
	}
	if got, ok := EnvelopeProcessingTime(contractsv1.Envelope{EventTime: event}); !ok || !got.Equal(event) {
		t.Fatalf("event fallback = %v ok=%v", got, ok)
	}
	if _, ok := EnvelopeProcessingTime(contractsv1.Envelope{}); ok {
		t.Fatal("zero envelope produced a processing time")
	}
}

func TestRecordProcessingTimeNormalizesToUTC(t *testing.T) {
	t.Parallel()
	ingested := time.Date(2026, 1, 2, 3, 0, 0, 0, time.FixedZone("offset", 3600))
	event := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := RecordProcessingTime(ingested, event); got != ingested.UTC() {
		t.Fatalf("processing time = %v want %v", got, ingested.UTC())
	}
	if got := RecordProcessingTime(time.Time{}, event); !got.Equal(event) {
		t.Fatalf("event fallback = %v", got)
	}
}
