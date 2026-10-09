package store

import (
	"strings"
	"testing"
	"time"
)

func publishFor(t *testing.T, s Store, situationID, entityID string, version int, evidence ...string) {
	t.Helper()
	published := publishedVersion(version)
	published.SituationID, published.EntityID = situationID, entityID
	if len(evidence) > 0 {
		published.Evidence = evidence
	}
	publish(t, s, published)
}

func TestListSituationsIsTenantScopedFilterableAndNewestEvidenceFirst(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	publishFor(t, s, "sit-old", "m1", 1)
	newer := publishedVersion(1)
	newer.SituationID, newer.EntityID, newer.EventHorizon = "sit-new", "m2", testNow.Add(time.Hour)
	publish(t, s, newer)
	reader := NewReader(s.db)

	all, err := reader.ListSituations(t.Context(), "tenant", "")
	if err != nil || len(all) != 2 || all[0].SituationID != "sit-new" || all[1].SituationID != "sit-old" {
		t.Fatalf("all = %+v err=%v, want sit-new then sit-old", all, err)
	}
	first := all[1]
	if first.EntityType != "motor" || first.EntityID != "m1" || first.SituationType != "bearing" || first.Phase != "watch" || first.Status != "active" ||
		first.CurrentVersion != 1 || first.OccurrenceID != "occ-1" || first.DeploymentID != s.deploymentID || first.FirstEventTime == "" || first.LatestEventTime == "" {
		t.Fatalf("summary = %+v", first)
	}
	if one, err := reader.ListSituations(t.Context(), "tenant", "m2"); err != nil || len(one) != 1 || one[0].SituationID != "sit-new" {
		t.Fatalf("entity filter = %+v err=%v, want sit-new only", one, err)
	}
	if none, err := reader.ListSituations(t.Context(), "other", ""); err != nil || len(none) != 0 {
		t.Fatalf("another tenant = %+v err=%v, want none", none, err)
	}
}

func TestSituationVersionReadsAnExplicitVersionOrTheCurrentOneWithItsEvidence(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	publishFor(t, s, "sit-1", "m1", 1, "evt-1")
	publishFor(t, s, "sit-1", "m1", 2, "evt-1", "evt-2")
	reader := NewReader(s.db)

	first, err := reader.SituationVersion(t.Context(), "tenant", "sit-1", 1)
	if err != nil || first.Version != 1 || first.PreviousVersion != nil || len(first.Evidence) != 1 || first.Evidence[0] != "evt-1" {
		t.Fatalf("version 1 = %+v err=%v, want no predecessor and one evidence id", first, err)
	}
	if first.SnapshotSHA256 != "sha256:"+strings.Repeat("0", 64) || string(first.Snapshot) != `{"v":1}` || first.Phase != "watch" || first.Completeness != "on_time" || first.DeploymentID != s.deploymentID {
		t.Fatalf("version 1 = %+v, want the stored snapshot, digest and phase", first)
	}
	current, err := reader.SituationVersion(t.Context(), "tenant", "sit-1", 0)
	if err != nil || current.Version != 2 || current.PreviousVersion == nil || *current.PreviousVersion != 1 || len(current.Evidence) != 2 || current.LineageID == first.LineageID {
		t.Fatalf("current = %+v err=%v, want version 2 citing version 1 and its own lineage", current, err)
	}
}

func TestSituationVersionRefusesUnknownTenantSituationAndVersion(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	publishFor(t, s, "sit-1", "m1", 1)
	reader := NewReader(s.db)
	for name, call := range map[string]struct {
		tenant, situation string
		version           int
	}{"other tenant": {"other", "sit-1", 1}, "unknown situation": {"tenant", "nope", 0}, "unknown version": {"tenant", "sit-1", 9}} {
		if _, err := reader.SituationVersion(t.Context(), call.tenant, call.situation, call.version); err == nil || !strings.Contains(err.Error(), "read situation") {
			t.Errorf("%s: err = %v, want read situation", name, err)
		}
	}
}

func TestSituationVersionRefusesUndecodableLineageReferences(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	publishFor(t, s, "sit-1", "m1", 1)
	if _, err := s.db.ExecContext(t.Context(), "UPDATE lineage_sets SET references_json = X'7B'"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewReader(s.db).SituationVersion(t.Context(), "tenant", "sit-1", 1); err == nil || !strings.Contains(err.Error(), "decode lineage") {
		t.Fatalf("err = %v, want decode lineage", err)
	}
}

func TestListSituationsNamesAQueryFailure(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	if _, err := s.db.ExecContext(t.Context(), "DROP TABLE situations"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewReader(s.db).ListSituations(t.Context(), "tenant", ""); err == nil || !strings.Contains(err.Error(), "list situations") {
		t.Fatalf("err = %v, want list situations", err)
	}
}
