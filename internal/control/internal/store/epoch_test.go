package store_test

import (
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestEpochStateRecordIsTerminalOnceKilled(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	tx := persistence.Autocommit()
	if _, found, err := tx.EpochState(t.Context(), "e", "epoch control"); err != nil || found {
		t.Fatalf("uncontrolled epoch found=%v err=%v", found, err)
	}
	for _, state := range []string{"draining", "killed", "draining"} {
		if err := tx.RecordEpochState(t.Context(), "e", state, instant); err != nil {
			t.Fatal(err)
		}
	}
	if state, found, err := tx.EpochState(t.Context(), "e", "epoch control"); err != nil || !found || state != "killed" {
		t.Fatalf("state=%q found=%v err=%v", state, found, err)
	}
}

func TestUnstartedReservedEpisodesListsOnlyAdmittedEpisodesOfTheEpochHoldingAReservation(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	persistence := store.New(db)
	seedEpisode(t, db, "unstarted", "e1", "admitted")
	seedEpisode(t, db, "unreserved", "e1", "admitted")
	seedEpisode(t, db, "running", "e1", "running")
	seedEpisode(t, db, "settled", "e1", "admitted")
	seedEpisode(t, db, "other epoch", "e2", "admitted")
	tx := persistence.Autocommit()
	for _, episode := range []string{"unstarted", "running", "settled", "other epoch"} {
		if err := tx.InsertReservation(t.Context(), episode, "tenant", 5, now); err != nil {
			t.Fatalf("reserve %s: %v", episode, err)
		}
	}
	if err := tx.RecordSettlement(t.Context(), "settled", 5, now); err != nil {
		t.Fatalf("settle: %v", err)
	}
	episodes, err := tx.UnstartedReservedEpisodes(t.Context(), "e1")
	if err != nil || !slices.Equal(episodes, []string{"unstarted"}) {
		t.Fatalf("unstarted reserved episodes = %v err=%v, want [unstarted]", episodes, err)
	}
}

func TestUnstartedReservedEpisodesIsEmptyWithoutEpisodes(t *testing.T) {
	t.Parallel()
	persistence, _ := openStore(t)
	episodes, err := persistence.Autocommit().UnstartedReservedEpisodes(t.Context(), "e")
	if err != nil || len(episodes) != 0 {
		t.Fatalf("episodes = %v err=%v", episodes, err)
	}
}

func TestSupersedeEpochEndsOnlyTheInFlightEpisodesOfTheEpoch(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	persistence := store.New(db)
	seedEpisode(t, db, "mine", "e1", "running")
	seedEpisode(t, db, "theirs", "e2", "running")
	if err := persistence.WithTx(t.Context(), func(tx *store.Tx) error { return tx.SupersedeEpoch(t.Context(), "e1", instant) }); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	lifecycles := map[string]string{}
	for _, episode := range []string{"mine", "theirs"} {
		var lifecycle string
		if err := db.QueryRowContext(t.Context(), "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episode).Scan(&lifecycle); err != nil {
			t.Fatalf("read %s: %v", episode, err)
		}
		lifecycles[episode] = lifecycle
	}
	if lifecycles["mine"] != "superseded" || lifecycles["theirs"] != "running" {
		t.Fatalf("lifecycles after superseding e1 = %v", lifecycles)
	}
}
