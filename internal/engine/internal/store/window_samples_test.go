package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func windowState(key string, window *operators.WindowState) *operators.PartitionState {
	blobs := map[string]*operators.OperatorStateBlob{key: {Window: window}}
	return &operators.PartitionState{OperatorStates: map[string]map[string]*operators.OperatorStateBlob{"op": blobs}}
}

func sample(id string, at time.Duration, value float64) operators.Sample {
	return operators.Sample{EventID: id, EventTime: testNow.Add(at), Value: value, BootID: "boot-A"}
}

func addSamples(window *operators.WindowState, cutoff time.Duration, samples ...operators.Sample) {
	for _, s := range samples {
		window.AddSample(s, testNow.Add(cutoff))
	}
}

func storedSampleIDs(t *testing.T, s Store) string {
	t.Helper()
	return queryStoreText(t, s, "SELECT COALESCE(group_concat(event_id), '') FROM (SELECT event_id FROM operator_window_samples ORDER BY event_time, event_id)")
}

func queryStoreText(t *testing.T, s Store, query string) string {
	t.Helper()
	var text string
	if err := s.db.QueryRowContext(t.Context(), query).Scan(&text); err != nil {
		t.Fatal(err)
	}
	return text
}

func TestWindowSamplesAreRowsOfTheirOwnAndTheBlobCarriesNone(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	window := &operators.WindowState{BootID: "boot-A"}
	addSamples(window, -time.Hour, sample("evt-2", 2*time.Minute, 2.5), sample("evt-0", 0, 0.25), sample("evt-1", time.Minute, 1.5))
	inTx(t, s, func(tx *Tx) error {
		return tx.SaveOperatorState(t.Context(), 0, "m1", windowState("m1", window), testNow)
	})
	if blob := queryStoreText(t, s, "SELECT CAST(state_blob AS TEXT) FROM operator_state"); strings.Contains(blob, "evt-") {
		t.Fatalf("state blob %s holds samples, want only the window position", blob)
	}
	inTx(t, s, func(tx *Tx) error {
		loaded, err := tx.LoadOperatorState(t.Context(), 0, "m1")
		if err != nil {
			return err
		}
		got := loaded.OperatorStates["op"]["m1"].Window.Samples
		want := []operators.Sample{sample("evt-0", 0, 0.25), sample("evt-1", time.Minute, 1.5), sample("evt-2", 2*time.Minute, 2.5)}
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("restored samples = %+v, want %+v in event-time order", got, want)
		}
		return nil
	})
}

func TestSavingAWindowWritesOnlyTheSamplesAddedAndExpiredSinceTheLastSave(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	window := &operators.WindowState{}
	addSamples(window, -time.Hour, sample("evt-0", 0, 1), sample("evt-1", time.Minute, 2))
	state := windowState("m1", window)
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", state, testNow) })
	addSamples(window, 30*time.Second, sample("evt-2", 2*time.Minute, 3))
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", state, testNow) })
	if got := storedSampleIDs(t, s); got != "evt-1,evt-2" {
		t.Fatalf("stored samples = %s, want evt-0 expired and evt-2 added", got)
	}
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", state, testNow) })
	if got := storedSampleIDs(t, s); got != "evt-1,evt-2" {
		t.Fatalf("stored samples = %s after a save with nothing new, want them unchanged", got)
	}
}

func TestASampleThatExpiresBeforeItsFirstSaveIsNeverWritten(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	window := &operators.WindowState{}
	addSamples(window, 0, sample("evt-late", -time.Hour, 9), sample("evt-now", time.Minute, 1))
	inTx(t, s, func(tx *Tx) error {
		return tx.SaveOperatorState(t.Context(), 0, "m1", windowState("m1", window), testNow)
	})
	if got := storedSampleIDs(t, s); got != "evt-now" {
		t.Fatalf("stored samples = %s, want only the sample inside the window", got)
	}
}

func TestRetiringAnOperatorStateRemovesItsSamples(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	kept, retired := &operators.WindowState{}, &operators.WindowState{}
	addSamples(kept, -time.Hour, sample("evt-kept", 0, 1))
	addSamples(retired, -time.Hour, sample("evt-retired", 0, 2))
	state := windowState("m1", kept)
	state.OperatorStates["op"]["m1\x1fboot-A"] = &operators.OperatorStateBlob{Window: retired}
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", state, testNow) })
	delete(state.OperatorStates["op"], "m1\x1fboot-A")
	inTx(t, s, func(tx *Tx) error { return tx.SaveOperatorState(t.Context(), 0, "m1", state, testNow) })
	if got := storedSampleIDs(t, s); got != "evt-kept" {
		t.Fatalf("stored samples = %s, want the retired state's samples removed with it", got)
	}
}

func TestWindowSamplesLoadPerEntityAndNeverFromANeighbor(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	for _, entity := range []string{"m1", "m10", "m2"} {
		window := &operators.WindowState{}
		addSamples(window, -time.Hour, sample("evt-"+entity, 0, 1))
		inTx(t, s, func(tx *Tx) error {
			return tx.SaveOperatorState(t.Context(), 0, entity, windowState(entity, window), testNow)
		})
	}
	inTx(t, s, func(tx *Tx) error {
		loaded, err := tx.LoadOperatorState(t.Context(), 0, "m1")
		if err != nil || len(loaded.OperatorStates["op"]) != 1 || len(loaded.OperatorStates["op"]["m1"].Window.Samples) != 1 {
			t.Fatalf("m1 scope = %+v err=%v, want only m1 with its one sample", loaded.OperatorStates, err)
		}
		return nil
	})
}

func TestWindowSampleStatementsNameTheirFailure(t *testing.T) {
	t.Parallel()
	save := func(ctx context.Context, tx *Tx) error {
		window := &operators.WindowState{}
		addSamples(window, -time.Hour, sample("evt-0", 0, 1))
		return tx.SaveOperatorState(ctx, 0, "m1", windowState("m1", window), testNow)
	}
	checkTxFailures(t, []txFailure{
		{"load", "operator_window_samples", func(ctx context.Context, tx *Tx) error { _, err := tx.LoadOperatorState(ctx, 0, "m1"); return err }, "query window samples"},
		{"insert", "operator_window_samples", save, "insert window sample evt-0"},
	})
}

func TestWindowSamplesLoadThroughThePrimaryKeyRangeOfTheEntity(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	storagetest.RequireIndexedPlan(t, db, "PRIMARY KEY (deployment_id=? AND tenant_id=? AND partition_id=? AND state_key>? AND state_key<?)",
		windowSamplesSQL, "deployment", "tenant", 0, "m1", "m1", "m1", "m1", "m1", "m1")
}
