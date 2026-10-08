package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func allowOwner(context.Context, *sql.Tx, string) error { return nil }

func openStore(t *testing.T) Store {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	compiled, err := spec.CompileFile(t.Context(), "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, allowOwner, "epoch", "tenant", compiled.Digest)
	if err := s.SaveDeployment(t.Context(), compiled); err != nil {
		t.Fatal(err)
	}
	return s
}

func inTx(t *testing.T, s Store, use func(*Tx) error) {
	t.Helper()
	if err := s.WithTx(t.Context(), use); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRequiresTheDatabaseAndOwnerCheck(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	if !s.Configured() || New(nil, allowOwner, "e", "t", "d").Configured() || New(s.db, nil, "e", "t", "d").Configured() {
		t.Fatal("store configuration check changed")
	}
}

func TestOwnerFailureRefusesTheUnitOfWork(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	lost := errors.New("ownership lost")
	denied := New(s.db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch", "tenant", s.deploymentID)
	if err := denied.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertOwner(t.Context()) }); !errors.Is(err, lost) {
		t.Fatalf("err = %v", err)
	}
}

func TestRecordedEventsAreIdempotentAndAdvanceTheCheckpoint(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	if checkpoint, err := s.LoadCheckpoint(t.Context(), 1); err != nil || checkpoint != (domain.Checkpoint{}) {
		t.Fatalf("fresh checkpoint = %+v err=%v", checkpoint, err)
	}
	inTx(t, s, func(tx *Tx) error {
		if applied, err := tx.EventApplied(t.Context(), "evt-1"); err != nil || applied {
			t.Fatalf("unseen event applied=%v err=%v", applied, err)
		}
		return tx.RecordApplied(t.Context(), 1, "evt-1", 7, testNow, testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		if applied, err := tx.EventApplied(t.Context(), "evt-1"); err != nil || !applied {
			t.Fatalf("recorded event applied=%v err=%v", applied, err)
		}
		return nil
	})
	checkpoint, err := s.LoadCheckpoint(t.Context(), 1)
	if err != nil || checkpoint.LastPosition != 7 || checkpoint.Watermark != testNow.Format(time.RFC3339Nano) {
		t.Fatalf("checkpoint = %+v err=%v", checkpoint, err)
	}
	if err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.RecordApplied(t.Context(), 1, "evt-1", 8, testNow, testNow) }); err == nil {
		t.Fatal("a duplicate inbox entry was accepted")
	}
}

func operatorState(keys ...string) *operators.PartitionState {
	blobs := make(map[string]*operators.OperatorStateBlob, len(keys))
	for _, key := range keys {
		blobs[key] = &operators.OperatorStateBlob{}
	}
	return &operators.PartitionState{OperatorStates: map[string]map[string]*operators.OperatorStateBlob{"op": blobs}}
}

func TestOperatorStateScopeIsAnEntityAndItsCompositeKeysOnly(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error {
		if err := tx.SaveOperatorState(t.Context(), 0, "m1", operatorState("m1", "m1\x1fa"), testNow); err != nil {
			return err
		}
		return tx.SaveOperatorState(t.Context(), 0, "m10", operatorState("m10"), testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		m1, err := tx.LoadOperatorState(t.Context(), 0, "m1")
		if err != nil || len(m1.OperatorStates["op"]) != 2 {
			t.Fatalf("m1 scope = %v err=%v", m1.OperatorStates, err)
		}
		all, err := tx.LoadOperatorState(t.Context(), 0, "")
		if err != nil || len(all.OperatorStates["op"]) != 3 {
			t.Fatalf("partition scope = %v err=%v", all.OperatorStates, err)
		}
		return tx.SaveOperatorState(t.Context(), 0, "m1", operatorState("m1"), testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		all, err := tx.LoadOperatorState(t.Context(), 0, "")
		if err != nil || len(all.OperatorStates["op"]) != 2 {
			t.Fatalf("saving m1 must replace only m1's scope: %v err=%v", all.OperatorStates, err)
		}
		return tx.SaveOperatorState(t.Context(), 0, "m1", nil, testNow)
	})
}

func publishedVersion(version int) situations.Version {
	return situations.Version{SituationID: "sit-1", Version: version, PreviousVersion: version - 1, Type: "bearing", EntityType: "motor", EntityID: "m1",
		Phase: "watch", EventHorizon: testNow, Watermark: testNow, Completeness: "on_time", Severity: 1, Confidence: 0.5,
		SnapshotJSON: []byte(`{"v":1}`), SnapshotSHA256: "sha256:" + string(repeat('0', 64)), Evidence: []string{"evt-1"}}
}

func repeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestSituationVersionsPersistWithLineageAndGuardRuntimeState(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	lineage, err := domain.NewLineage([]string{"evt-1"})
	if err != nil {
		t.Fatal(err)
	}
	version := publishedVersion(1)
	write := domain.SituationWrite{OccurrenceID: "occ-1", FirstEventTime: testNow, StateJSON: []byte(`{}`), StateDigest: make([]byte, 32)}
	inTx(t, s, func(tx *Tx) error {
		for range 2 {
			if err := tx.RecordLineage(t.Context(), lineage, testNow); err != nil {
				return err
			}
		}
		if err := tx.UpsertSituation(t.Context(), 0, version, write, testNow); err != nil {
			return err
		}
		return tx.InsertSituationVersion(t.Context(), version, lineage.ID, testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		current := situations.Situation{SituationID: "sit-1", Version: 1, Phase: "alert", LatestEventTime: testNow}
		if err := tx.SaveSituationRuntimeState(t.Context(), current, []byte(`{"a":1}`), make([]byte, 32), testNow); err != nil {
			return err
		}
		stale := current
		stale.Version = 5
		if err := tx.SaveSituationRuntimeState(t.Context(), stale, []byte(`{}`), make([]byte, 32), testNow); err == nil {
			t.Fatal("a state write for a diverged version was accepted")
		}
		return nil
	})
	var restored []domain.StoredSituation
	if err := s.EachCurrentSituation(t.Context(), func(r domain.StoredSituation) error { restored = append(restored, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || restored[0].Phase != "alert" || restored[0].Version != 1 || restored[0].StateCodecVersion != 1 {
		t.Fatalf("restored = %+v", restored)
	}
	stop := errors.New("stop")
	if err := s.EachCurrentSituation(t.Context(), func(domain.StoredSituation) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("restore callback error = %v", err)
	}
}

func TestVersionInsertRefusesAnInvalidSnapshotDigest(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	version := publishedVersion(1)
	version.SnapshotSHA256 = "bad"
	err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.InsertSituationVersion(t.Context(), version, "lin", testNow) })
	if err == nil {
		t.Fatal("invalid snapshot digest accepted")
	}
}

func TestHeartbeatTimersAreReplacedAndAcknowledgedOnce(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	due := testNow.Add(time.Minute)
	timer := domain.HeartbeatTimer{ID: "tmr-1", OperatorID: "hb", StateKey: "m1", DueAt: due, Payload: []byte(`{"expected_event_id":"evt-1"}`)}
	inTx(t, s, func(tx *Tx) error {
		if err := tx.ArmHeartbeatTimer(t.Context(), 0, timer, testNow); err != nil {
			return err
		}
		return tx.ArmHeartbeatTimer(t.Context(), 0, timer, testNow)
	})
	if partitions, err := s.TimerPartitions(t.Context()); err != nil || len(partitions) != 1 || partitions[0] != 0 {
		t.Fatalf("partitions = %v err=%v", partitions, err)
	}
	inTx(t, s, func(tx *Tx) error {
		if timers, err := tx.LoadDueTimers(t.Context(), 0, testNow); err != nil || len(timers) != 0 {
			t.Fatalf("a timer before its due time loaded: %v err=%v", timers, err)
		}
		timers, err := tx.LoadDueTimers(t.Context(), 0, due)
		if err != nil || len(timers) != 1 || timers[0].ExpectedEventID != "evt-1" || timers[0].StateKey != "m1" {
			t.Fatalf("due timers = %+v err=%v", timers, err)
		}
		return tx.AcknowledgeTimers(t.Context(), timers, due)
	})
	inTx(t, s, func(tx *Tx) error {
		if timers, err := tx.LoadDueTimers(t.Context(), 0, due); err != nil || len(timers) != 0 {
			t.Fatalf("a fired timer loaded again: %v err=%v", timers, err)
		}
		return tx.ArmHeartbeatTimer(t.Context(), 0, timer, testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		if timers, err := tx.LoadDueTimers(t.Context(), 0, due); err != nil || len(timers) != 0 {
			t.Fatalf("re-arming must never revive a fired timer: %v err=%v", timers, err)
		}
		return nil
	})
}

func TestCorruptTimerPayloadRefusesTheBatch(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	bad := domain.HeartbeatTimer{ID: "tmr-bad", OperatorID: "hb", StateKey: "m1", DueAt: testNow, Payload: []byte("not json")}
	inTx(t, s, func(tx *Tx) error { return tx.ArmHeartbeatTimer(t.Context(), 0, bad, testNow) })
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		_, err := tx.LoadDueTimers(t.Context(), 0, testNow)
		return err
	})
	if err == nil {
		t.Fatal("a timer with an undecodable payload loaded")
	}
}

func TestFailedUnitOfWorkRollsBackAndWALCheckpointSucceeds(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	boom := errors.New("boom")
	if err := s.WithTx(t.Context(), func(tx *Tx) error {
		if err := tx.RecordApplied(t.Context(), 2, "evt-x", 1, testNow, testNow); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	inTx(t, s, func(tx *Tx) error {
		if applied, err := tx.EventApplied(t.Context(), "evt-x"); err != nil || applied {
			t.Fatalf("rolled-back event applied=%v err=%v", applied, err)
		}
		return nil
	})
	if err := s.CheckpointWAL(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := s.RetryBusy(t.Context(), func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
