package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
)

func TestPublishedVersionsAreReadableByNumberAndAsTheCurrentOne(t *testing.T) {
	t.Parallel()
	rig := newRig(t, escalationSpec())
	appendLevel(t, rig.log, "evt-1", 0, 15)
	runGlobal(t, rig.service)
	appendLevel(t, rig.log, "evt-2", time.Minute, 60)
	runGlobal(t, rig.service)
	reader := store.NewReader(rig.db)

	listed, err := app.ListSituations(t.Context(), reader, "default", "")
	if err != nil || len(listed) != 1 || listed[0].CurrentVersion != 2 || listed[0].EntityID != "thing-1" {
		t.Fatalf("listed = %+v err=%v, want one situation of thing-1 at version 2", listed, err)
	}
	id := listed[0].SituationID
	current, err := app.SituationVersion(t.Context(), reader, "default", id, 0)
	if err != nil || current.Version != 2 || current.PreviousVersion == nil || *current.PreviousVersion != 1 || !strings.HasPrefix(current.LineageID, "lin_") {
		t.Fatalf("current = %+v err=%v, want version 2 citing version 1 and a lineage", current, err)
	}
	earlier, err := app.SituationVersion(t.Context(), reader, "default", id, 1)
	if err != nil || earlier.Version != 1 || !strings.HasPrefix(earlier.SnapshotSHA256, "sha256:") || earlier.Phase != "watch" || current.Phase != "warning" {
		t.Fatalf("earlier = %+v err=%v, want the immutable version 1 in phase watch with a snapshot digest", earlier, err)
	}
	if none, err := app.ListSituations(t.Context(), reader, "default", "no-such-entity"); err != nil || len(none) != 0 {
		t.Fatalf("filtered = %+v err=%v, want none", none, err)
	}
}

func TestReadsNameTheTenantAndSituationTheyFailedFor(t *testing.T) {
	t.Parallel()
	rig := newRig(t, restartSpec())
	reader := store.NewReader(rig.db)
	if _, err := app.SituationVersion(t.Context(), reader, "default", "sit-missing", 3); err == nil || !strings.Contains(err.Error(), "situation sit-missing version 3") {
		t.Fatalf("err = %v, want situation sit-missing version 3", err)
	}
	if err := rig.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ListSituations(t.Context(), reader, "default", ""); err == nil || !strings.Contains(err.Error(), "situations of tenant default") {
		t.Fatalf("err = %v, want situations of tenant default", err)
	}
}
