package domain

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func storedSituation(t *testing.T) StoredSituation {
	t.Helper()
	state := SituationState{SituationID: "sit-1", OccurrenceID: "occ-1", PartitionID: 3, Version: 2,
		Facts: map[string]any{"level": 4.5, "level_event_time": kernel.FormatTime(base)}, Evidence: []string{"evt-1", "evt-2"}}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(stateJSON, &document); err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	return StoredSituation{SituationID: "sit-1", Type: "bearing", EntityType: "motor", EntityID: "motor-1", OccurrenceID: "occ-1", Phase: "watch",
		PartitionID: 3, Version: 2, Severity: 1, StateCodecVersion: 1, FirstEventTime: base,
		LatestEventTime: base, UpdatedAt: base, Completeness: "complete",
		StateJSON: stateJSON, StateSHA256: raw, PreviousPhase: "normal", Confidence: 0.5}
}

func TestRestoreRebuildsTheSituationFromVerifiedState(t *testing.T) {
	t.Parallel()
	situation, err := storedSituation(t).Restore("tenant", "deployment")
	if err != nil {
		t.Fatal(err)
	}
	if situation.TenantID != "tenant" || situation.DeploymentID != "deployment" || situation.Version != 2 || situation.PreviousPhase != "normal" ||
		len(situation.Evidence) != 2 || !situation.FirstEventTime.Equal(base) {
		t.Fatalf("situation = %+v", situation)
	}
	if got, ok := situation.Facts["level_event_time"].(time.Time); !ok || !got.Equal(base) {
		t.Fatalf("fact time not restored: %#v", situation.Facts["level_event_time"])
	}
}

func TestRestoreRefusesUnsafeStoredState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*StoredSituation)
		want   string
	}{
		{"legacy codec needs rebuild", func(r *StoredSituation) { r.StateCodecVersion = 0 }, "requires rebuild"},
		{"unknown codec", func(r *StoredSituation) { r.StateCodecVersion = 7 }, "unsupported state codec 7"},
		{"empty state", func(r *StoredSituation) { r.StateJSON = nil }, "incomplete persisted state"},
		{"short digest", func(r *StoredSituation) { r.StateSHA256 = make([]byte, 4) }, "incomplete persisted state"},
		{"digest mismatch", func(r *StoredSituation) { r.StateSHA256 = make([]byte, sha256.Size) }, "digest mismatch"},
		{"identity mismatch", func(r *StoredSituation) { r.PartitionID = 9 }, "identity mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stored := storedSituation(t)
			tc.mutate(&stored)
			if _, err := stored.Restore("tenant", "deployment"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
