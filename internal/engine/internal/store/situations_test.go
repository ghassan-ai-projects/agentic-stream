package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func TestSituationVersionsPersistWithLineageAndGuardRuntimeState(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	lineage, err := domain.NewLineage([]string{"evt-1"})
	if err != nil {
		t.Fatal(err)
	}
	inTx(t, s, func(tx *Tx) error {
		for range 2 {
			if err := tx.RecordLineage(t.Context(), lineage, testNow); err != nil {
				return err
			}
		}
		return nil
	})
	var lineages int
	if err := s.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM lineage_sets WHERE lineage_id = ?", lineage.ID).Scan(&lineages); err != nil || lineages != 1 {
		t.Fatalf("lineage rows = %d err=%v, want one however often the same evidence set is recorded", lineages, err)
	}
	publish(t, s, publishedVersion(1))
	inTx(t, s, func(tx *Tx) error {
		current := situations.Situation{SituationID: "sit-1", Version: 1, Phase: "alert", LatestEventTime: testNow}
		return tx.SaveSituationRuntimeState(t.Context(), current, []byte(`{"a":1}`), make([]byte, 32), testNow)
	})
	restored := eachCurrentSituation(t, s)
	if len(restored) != 1 || restored[0].Phase != "alert" || restored[0].Version != 1 || restored[0].StateCodecVersion != 1 {
		t.Fatalf("restored = %+v, want one situation at version 1 in phase alert with codec 1", restored)
	}
	if !restored[0].FirstEventTime.Equal(testNow) || !restored[0].LatestEventTime.Equal(testNow) || !restored[0].UpdatedAt.Equal(testNow) {
		t.Fatalf("restored times = %v %v %v, want testNow", restored[0].FirstEventTime, restored[0].LatestEventTime, restored[0].UpdatedAt)
	}
}

func TestRuntimeStateOfADivergedVersionIsRefused(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	publish(t, s, publishedVersion(1))
	stale := situations.Situation{SituationID: "sit-1", Version: 5, Phase: "alert", LatestEventTime: testNow}
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		return tx.SaveSituationRuntimeState(t.Context(), stale, []byte(`{}`), make([]byte, 32), testNow)
	})
	if err == nil || !strings.Contains(err.Error(), "current_version diverged") {
		t.Fatalf("err = %v, want current_version diverged", err)
	}
}

func TestAPublishedSituationVersionIsNeverRewritten(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	first := publishedVersion(1)
	publish(t, s, first)
	snapshotOf := func() string {
		var snapshot string
		if err := s.db.QueryRowContext(t.Context(), "SELECT CAST(snapshot_json AS TEXT) FROM situation_versions WHERE situation_id = 'sit-1' AND version = 1").Scan(&snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	before := snapshotOf()

	rewrite := first
	rewrite.SnapshotJSON = []byte(`{"v":"rewritten"}`)
	lineage, err := domain.NewLineage(rewrite.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	err = s.WithTx(t.Context(), func(tx *Tx) error { return tx.InsertSituationVersion(t.Context(), rewrite, lineage.ID, testNow) })
	if err == nil || !strings.Contains(err.Error(), "insert situation version") {
		t.Fatalf("err = %v, want the second insert of version 1 refused", err)
	}
	second := publishedVersion(2)
	second.SnapshotJSON = []byte(`{"v":2}`)
	publish(t, s, second)
	inTx(t, s, func(tx *Tx) error {
		current := situations.Situation{SituationID: "sit-1", Version: 2, Phase: "alert", LatestEventTime: testNow}
		return tx.SaveSituationRuntimeState(t.Context(), current, []byte(`{}`), make([]byte, 32), testNow)
	})
	if after := snapshotOf(); after != before {
		t.Fatalf("version 1 snapshot changed from %s to %s", before, after)
	}
}

func TestInsertedVersionRecordsItsPredecessorOnlyWhenItHasOne(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	publish(t, s, publishedVersion(1))
	publish(t, s, publishedVersion(2))
	previous := map[int]*int{}
	rows, err := s.db.QueryContext(t.Context(), "SELECT version, previous_version FROM situation_versions ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var version int
		var prev *int
		if err := rows.Scan(&version, &prev); err != nil {
			t.Fatal(err)
		}
		previous[version] = prev
	}
	if previous[1] != nil || previous[2] == nil || *previous[2] != 1 {
		t.Fatalf("previous versions = %v, want none for version 1 and 1 for version 2", previous)
	}
}

func TestVersionInsertRefusesAnInvalidSnapshotDigest(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	version := publishedVersion(1)
	version.SnapshotSHA256 = "bad"
	err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.InsertSituationVersion(t.Context(), version, "lin", testNow) })
	if err == nil || !strings.Contains(err.Error(), "invalid snapshot digest") {
		t.Fatalf("err = %v, want invalid snapshot digest", err)
	}
}

func TestEachCurrentSituationRestoresOnlyTheDeploymentsTenantInEntityOrder(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	for _, entity := range []struct{ situation, entity string }{{"sit-b", "m2"}, {"sit-a", "m1"}} {
		version := publishedVersion(1)
		version.SituationID, version.EntityID = entity.situation, entity.entity
		publish(t, s, version)
	}
	elsewhere := New(s.db, allowOwner, "epoch", "other", s.deploymentID)
	if got := len(eachCurrentSituation(t, elsewhere)); got != 0 {
		t.Fatalf("another tenant restored %d situations, want none", got)
	}
	restored := eachCurrentSituation(t, s)
	if len(restored) != 2 || restored[0].EntityID != "m1" || restored[1].EntityID != "m2" {
		t.Fatalf("restored = %+v, want m1 then m2", restored)
	}
	stop := errors.New("stop")
	if err := s.EachCurrentSituation(t.Context(), func(domain.StoredSituation) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("restore callback error = %v, want the callback's error", err)
	}
}

func TestCorruptStoredTimesRefuseTheReadWithTheirColumn(t *testing.T) {
	t.Parallel()
	for column, want := range map[string]string{
		"first_event_time":  "parse first event time of situation sit-1",
		"latest_event_time": "parse latest event time of situation sit-1",
		"updated_at":        "parse updated time of situation sit-1",
	} {
		t.Run(column, func(t *testing.T) {
			t.Parallel()
			s := openStore(t)
			publish(t, s, publishedVersion(1))
			if _, err := s.db.ExecContext(t.Context(), "UPDATE situations SET "+column+" = 'not a time'"); err != nil {
				t.Fatal(err)
			}
			err := s.EachCurrentSituation(t.Context(), func(domain.StoredSituation) error { return nil })
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestSituationStatementsNameTheirFailure(t *testing.T) {
	t.Parallel()
	lineage, err := domain.NewLineage([]string{"evt-1"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []txFailure{
		{"lineage", "lineage_sets", func(ctx context.Context, tx *Tx) error { return tx.RecordLineage(ctx, lineage, testNow) }, "insert lineage set"},
		{"situation", "situations", func(ctx context.Context, tx *Tx) error {
			return tx.UpsertSituation(ctx, 0, publishedVersion(1), publishWrite(), testNow)
		}, "upsert situation"},
		{"version", "situation_versions", func(ctx context.Context, tx *Tx) error {
			return tx.InsertSituationVersion(ctx, publishedVersion(1), lineage.ID, testNow)
		}, "insert situation version"},
		{"runtime state", "situations", func(ctx context.Context, tx *Tx) error {
			return tx.SaveSituationRuntimeState(ctx, situations.Situation{SituationID: "sit-1", Version: 1}, []byte(`{}`), make([]byte, 32), testNow)
		}, "update situation runtime state"},
	}
	checkTxFailures(t, cases)
}

func eachCurrentSituation(t *testing.T, s Store) []domain.StoredSituation {
	t.Helper()
	var restored []domain.StoredSituation
	err := s.EachCurrentSituation(t.Context(), func(r domain.StoredSituation) error { restored = append(restored, r); return nil })
	if err != nil {
		t.Fatal(err)
	}
	return restored
}
