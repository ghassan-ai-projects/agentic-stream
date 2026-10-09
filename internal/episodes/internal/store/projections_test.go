package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func inTx(t *testing.T, db *storage.DB, work func(tx *store.Tx) error) error {
	t.Helper()
	return db.WithTx(t.Context(), func(raw *sql.Tx) error { return work(store.Join(raw)) })
}

func TestDispatchableEpisodeReadsTheOldestAdmittedEpisodeOfTheTenant(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	var found bool
	var episodeID, tenant string
	var rebinds int
	if err := inTx(t, db, func(tx *store.Tx) error {
		episode, ok, err := store.DispatchableEpisode(t.Context(), tx, "default", false)
		episodeID, tenant, rebinds, found = episode.EpisodeID, episode.TenantID, episode.StaleRebindCount, ok
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !found || episodeID != admittedEpisodeID(t, db) || tenant != "default" || rebinds != 0 {
		t.Fatalf("dispatchable episode = (%q, tenant %q, rebinds %d, found %v), want the admitted episode of tenant default", episodeID, tenant, rebinds, found)
	}
	if err := inTx(t, db, func(tx *store.Tx) error {
		_, found, err := store.DispatchableEpisode(t.Context(), tx, "other-tenant", false)
		if found {
			t.Fatal("another tenant's dispatch produced an episode")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchableEpisodeNamesTheReadWhenItFails(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		_, _, err := store.DispatchableEpisode(ctx, store.Join(raw), "default", false)
		return err
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want one wrapping context.Canceled", err)
	}
}

func TestSchedulerItemEvaluationSnapshotAndLiveVersionAreReadByIdentity(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	itemID := scalar[string](t, db, "SELECT scheduler_item_id FROM scheduler_items LIMIT 1")
	if err := inTx(t, db, func(tx *store.Tx) error {
		item, err := store.LoadSchedulerItem(t.Context(), tx, itemID)
		if err != nil {
			return err
		}
		if item.SchedulerItemID != itemID || item.TenantID != "default" || item.SituationID == "" || item.TriggerID == "" || item.SituationVersion < 1 {
			t.Fatalf("scheduler item = %+v", item)
		}
		evaluation, err := store.LoadEvaluation(t.Context(), tx, item.TriggerID)
		if err != nil || evaluation.TriggerID != item.TriggerID || evaluation.TriggerName == "" {
			t.Fatalf("evaluation = %+v err = %v", evaluation, err)
		}
		snapshot, digest, _, _, err := store.LoadSnapshot(t.Context(), tx, item.SituationID, item.SituationVersion)
		if err != nil || len(snapshot) == 0 || len(digest) != 32 {
			t.Fatalf("snapshot = %d bytes, digest %d bytes, err = %v", len(snapshot), len(digest), err)
		}
		live, err := store.LiveSituationVersion(t.Context(), tx, item.TenantID, item.SituationID)
		if err != nil || live < int64(item.SituationVersion) {
			t.Fatalf("live version = %d (item bound to %d), err = %v", live, item.SituationVersion, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReadsOfAnUnknownRowReportNoRows(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	reads := map[string]func(tx *store.Tx) error{
		"scheduler item": func(tx *store.Tx) error { _, err := store.LoadSchedulerItem(t.Context(), tx, "missing"); return err },
		"evaluation":     func(tx *store.Tx) error { _, err := store.LoadEvaluation(t.Context(), tx, "missing"); return err },
		"snapshot": func(tx *store.Tx) error {
			_, _, _, _, err := store.LoadSnapshot(t.Context(), tx, "missing", 1)
			return err
		},
		"live version": func(tx *store.Tx) error {
			_, err := store.LiveSituationVersion(t.Context(), tx, "default", "missing")
			return err
		},
		"episode lifecycle": func(tx *store.Tx) error { _, err := store.EpisodeLifecycle(t.Context(), tx, "missing"); return err },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := inTx(t, db, read); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("error = %v, want sql.ErrNoRows", err)
			}
		})
	}
}

func TestProjectionErrorsPreserveCancellation(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	operations := map[string]func(tx *store.Tx) error{
		"evaluation": func(tx *store.Tx) error { _, err := store.LoadEvaluation(ctx, tx, "trigger"); return err },
		"snapshot": func(tx *store.Tx) error {
			_, _, _, _, err := store.LoadSnapshot(ctx, tx, "sit", 1)
			return err
		},
		"live version":      func(tx *store.Tx) error { _, err := store.LiveSituationVersion(ctx, tx, "tenant", "sit"); return err },
		"episode lifecycle": func(tx *store.Tx) error { _, err := store.EpisodeLifecycle(ctx, tx, "epi"); return err },
		"failed attempts":   func(tx *store.Tx) error { _, err := store.CountFailedAttempts(ctx, tx, "epi"); return err },
		"annotate":          func(tx *store.Tx) error { return store.AnnotateRejectedDecision(ctx, tx, "dec", "schema_invalid") },
		"accept":            func(tx *store.Tx) error { return store.AcceptDecision(ctx, tx, "dec") },
		"insert decision":   func(tx *store.Tx) error { return store.InsertDecision(ctx, tx, store.DecisionInsert{EpisodeID: "epi"}) },
		"insert intent": func(tx *store.Tx) error {
			return store.InsertValidatedIntent(ctx, tx, store.ValidatedIntentInsert{Intent: intentFixture()})
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := inTx(t, db, operation); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want one wrapping context.Canceled", err)
			}
		})
	}
}
