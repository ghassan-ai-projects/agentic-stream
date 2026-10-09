package domain

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func TestNewSituationWriteRequiresStateAndDefaultsIdentity(t *testing.T) {
	t.Parallel()
	if _, err := NewSituationWrite(situations.Version{SituationID: "sit-1", Version: 1}); err == nil || !strings.Contains(err.Error(), "has no runtime state") {
		t.Fatalf("missing state err = %v", err)
	}
	stored := storedSituation(t)
	digest := "sha256:" + hex.EncodeToString(stored.StateSHA256)
	version := situations.Version{SituationID: "sit-1", Version: 1, StateJSON: stored.StateJSON, StateSHA256: digest, EventHorizon: base}
	write, err := NewSituationWrite(version)
	if err != nil || write.OccurrenceID != "occ-sit-1" || !write.FirstEventTime.Equal(base) || len(write.StateDigest) != 32 {
		t.Fatalf("write = %+v err=%v", write, err)
	}
	version.OccurrenceID, version.FirstEventTime = "occ-9", base.Add(-1)
	if write, err := NewSituationWrite(version); err != nil || write.OccurrenceID != "occ-9" || !write.FirstEventTime.Equal(base.Add(-1)) {
		t.Fatalf("explicit identity lost: %+v err=%v", write, err)
	}
}

func TestNewSituationWriteDigestsStateWhenTheVersionCarriesNone(t *testing.T) {
	t.Parallel()
	stored := storedSituation(t)
	write, err := NewSituationWrite(situations.Version{SituationID: "sit-1", Version: 1, StateJSON: stored.StateJSON})
	if err != nil || string(write.StateDigest) != string(stored.StateSHA256) {
		t.Fatalf("derived digest = %x err=%v, want %x", write.StateDigest, err, stored.StateSHA256)
	}
	if _, err := NewSituationWrite(situations.Version{SituationID: "sit-1", Version: 1, StateJSON: []byte("{")}); err == nil {
		t.Fatal("undecodable state produced a digest")
	}
	if _, err := DecodeStateDigest("nope"); err == nil || !strings.Contains(err.Error(), "invalid situation state digest") {
		t.Fatalf("err = %v", err)
	}
	if _, err := DecodeSnapshotDigest("nope"); err == nil || !strings.Contains(err.Error(), "invalid snapshot digest") {
		t.Fatalf("err = %v", err)
	}
}

func TestPreviousVersionIsAbsentForAFirstVersion(t *testing.T) {
	t.Parallel()
	if _, ok := PreviousVersion(situations.Version{PreviousVersion: 0}); ok {
		t.Fatal("a first version has no previous version")
	}
	if previous, ok := PreviousVersion(situations.Version{PreviousVersion: 4}); !ok || previous != 4 {
		t.Fatalf("previous = %d, %v", previous, ok)
	}
}
