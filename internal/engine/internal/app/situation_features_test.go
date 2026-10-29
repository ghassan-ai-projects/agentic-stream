package app_test

import (
	"testing"
	"time"
)

func TestSituationFeaturesFollowTheLatestAppliedEventNotTheLastPublishedVersion(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	appendLevel(t, rig.log, "evt-1", 0, 15)
	runGlobal(t, rig.service)
	situationID := queryText(t, rig.db, "SELECT situation_id FROM situations")
	published := countRows(t, rig.db, "SELECT COUNT(*) FROM situation_versions")
	appendLevel(t, rig.log, "evt-2", 10*time.Second, 25)
	runGlobal(t, rig.service)

	features, found, err := rig.service.SituationFeatures(t.Context(), situationID)
	if err != nil || !found {
		t.Fatalf("SituationFeatures = %v, found %v, %v; want the situation's features", features, found, err)
	}
	if features["level"] != float64(25) {
		t.Fatalf("features = %v, want level 25 from the latest event", features)
	}
	if countRows(t, rig.db, "SELECT COUNT(*) FROM situation_versions") != published {
		t.Fatalf("the second event published a version; the test needs the runtime state to run ahead of the versions")
	}
}

func TestSituationFeaturesOfAnUnknownSituationAreNotFound(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	if features, found, err := rig.service.SituationFeatures(t.Context(), "sit-missing"); found || err != nil {
		t.Fatalf("SituationFeatures = %v, found %v, %v; want not found", features, found, err)
	}
}
