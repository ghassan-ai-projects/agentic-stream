package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
)

func TestStoreRequiresTheDatabaseAndOwnerCheck(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	cases := map[string]struct {
		store Store
		want  bool
	}{
		"complete":   {s, true},
		"no db":      {New(nil, allowOwner, "e", "t", "d"), false},
		"no owner":   {New(s.db, nil, "e", "t", "d"), false},
		"neither":    {New(nil, nil, "e", "t", "d"), false},
		"zero value": {Store{}, false},
	}
	for name, tc := range cases {
		if got := tc.store.Configured(); got != tc.want {
			t.Errorf("%s: Configured() = %v, want %v", name, got, tc.want)
		}
	}
}

func TestOwnerFailureRefusesTheUnitOfWork(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	lost := errors.New("ownership lost")
	denied := New(s.db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch", "tenant", s.deploymentID)
	err := denied.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertOwner(t.Context()) })
	if !errors.Is(err, lost) || !strings.Contains(err.Error(), "stream runtime ownership lost") {
		t.Fatalf("err = %v, want stream runtime ownership lost wrapping the owner error", err)
	}
}

func TestOwnerCheckReceivesTheEpochAndTheOpenTransaction(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	var gotEpoch string
	var gotTx *sql.Tx
	recording := New(s.db, func(_ context.Context, tx *sql.Tx, epoch string) error { gotEpoch, gotTx = epoch, tx; return nil }, "epoch-7", "tenant", s.deploymentID)
	inTx(t, recording, func(tx *Tx) error { return tx.AssertOwner(t.Context()) })
	if gotEpoch != "epoch-7" || gotTx == nil {
		t.Fatalf("owner check saw epoch %q tx %v, want epoch-7 and the unit's transaction", gotEpoch, gotTx)
	}
}

func TestRecordedEventsAreIdempotentAndAdvanceTheCheckpoint(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	if checkpoint, err := s.LoadCheckpoint(t.Context(), 1); err != nil || !reflect.DeepEqual(checkpoint, domain.Checkpoint{}) {
		t.Fatalf("fresh checkpoint = %+v err=%v", checkpoint, err)
	}
	inTx(t, s, func(tx *Tx) error {
		if applied, err := tx.EventApplied(t.Context(), "evt-1"); err != nil || applied {
			t.Fatalf("unseen event applied=%v err=%v", applied, err)
		}
		return tx.RecordApplied(t.Context(), 1, "evt-1", 7, domain.PartitionClock{Watermark: testNow}, testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		if applied, err := tx.EventApplied(t.Context(), "evt-1"); err != nil || !applied {
			t.Fatalf("recorded event applied=%v err=%v", applied, err)
		}
		return nil
	})
	checkpoint, err := s.LoadCheckpoint(t.Context(), 1)
	if err != nil || checkpoint.LastPosition != 7 || !checkpoint.Watermark.Equal(testNow) {
		t.Fatalf("checkpoint = %+v err=%v", checkpoint, err)
	}
	err = s.WithTx(t.Context(), func(tx *Tx) error {
		return tx.RecordApplied(t.Context(), 1, "evt-1", 8, domain.PartitionClock{Watermark: testNow}, testNow)
	})
	if err == nil || !strings.Contains(err.Error(), "mark inbox") {
		t.Fatalf("err = %v, want mark inbox: a duplicate inbox entry must be refused", err)
	}
}

func TestCheckpointAdvancesPerPartitionAndAppliedThroughIsTheGreatestPosition(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	if position, err := s.AppliedThrough(t.Context()); err != nil || position != 0 {
		t.Fatalf("fresh applied position = %d err=%v, want 0", position, err)
	}
	inTx(t, s, func(tx *Tx) error {
		for i, partition := range []int{1, 2, 1} {
			position := int64([]int{5, 9, 7}[i])
			if err := tx.RecordApplied(t.Context(), partition, []string{"evt-1", "evt-2", "evt-3"}[i], position, domain.PartitionClock{Watermark: testNow.Add(time.Duration(i) * time.Minute)}, testNow); err != nil {
				return err
			}
		}
		return nil
	})
	for partition, want := range map[int]int64{1: 7, 2: 9, 3: 0} {
		if checkpoint, err := s.LoadCheckpoint(t.Context(), partition); err != nil || checkpoint.LastPosition != want {
			t.Errorf("partition %d checkpoint = %+v err=%v, want position %d", partition, checkpoint, err, want)
		}
	}
	if position, err := s.AppliedThrough(t.Context()); err != nil || position != 9 {
		t.Fatalf("applied position = %d err=%v, want 9", position, err)
	}
}

func TestInboxAndCheckpointAreScopedToTheTenant(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error {
		return tx.RecordApplied(t.Context(), 1, "evt-1", 7, domain.PartitionClock{Watermark: testNow}, testNow)
	})
	other := New(s.db, allowOwner, "epoch", "other", s.deploymentID)
	inTx(t, other, func(tx *Tx) error {
		if applied, err := tx.EventApplied(t.Context(), "evt-1"); err != nil || applied {
			t.Fatalf("another tenant sees the event applied=%v err=%v", applied, err)
		}
		return nil
	})
	if position, err := other.AppliedThrough(t.Context()); err != nil || position != 0 {
		t.Fatalf("another tenant's applied position = %d err=%v, want 0", position, err)
	}
}

func TestFailedUnitOfWorkRollsBackWholeAndWALCheckpointSucceeds(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	boom := errors.New("boom")
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		if err := tx.RecordApplied(t.Context(), 2, "evt-x", 1, domain.PartitionClock{Watermark: testNow}, testNow); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the work error", err)
	}
	inTx(t, s, func(tx *Tx) error {
		if applied, err := tx.EventApplied(t.Context(), "evt-x"); err != nil || applied {
			t.Fatalf("rolled-back event applied=%v err=%v", applied, err)
		}
		return nil
	})
	if checkpoint, err := s.LoadCheckpoint(t.Context(), 2); err != nil || checkpoint.LastPosition != 0 {
		t.Fatalf("rolled-back checkpoint = %+v err=%v, want untouched", checkpoint, err)
	}
}

func TestRetryBusyRunsTheWorkOnceWhenItSucceeds(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	calls := 0
	if err := s.RetryBusy(t.Context(), func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("calls = %d err=%v, want one call", calls, err)
	}
	stop := errors.New("not busy")
	calls = 0
	if err := s.RetryBusy(t.Context(), func() error { calls++; return stop }); !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("calls = %d err=%v, want a non-busy error returned after one call", calls, err)
	}
}

func TestCorruptCheckpointWatermarkRefusesTheRead(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error {
		return tx.RecordApplied(t.Context(), 1, "evt-1", 7, domain.PartitionClock{Watermark: testNow}, testNow)
	})
	if _, err := s.db.ExecContext(t.Context(), "UPDATE partition_checkpoints SET watermark = 'not a time'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadCheckpoint(t.Context(), 1); err == nil || !strings.Contains(err.Error(), "parse checkpoint watermark of partition 1") {
		t.Fatalf("err = %v, want parse checkpoint watermark of partition 1", err)
	}
}

func TestCheckpointStatementsNameTheirFailure(t *testing.T) {
	t.Parallel()
	cases := []txFailure{
		{"inbox check", "event_inbox", func(ctx context.Context, tx *Tx) error { _, err := tx.EventApplied(ctx, "evt-1"); return err }, "check inbox"},
		{"checkpoint update", "partition_checkpoints", func(ctx context.Context, tx *Tx) error {
			return tx.RecordApplied(ctx, 1, "evt-1", 7, domain.PartitionClock{Watermark: testNow}, testNow)
		}, "update checkpoint"},
		{"inbox insert", "event_inbox", func(ctx context.Context, tx *Tx) error {
			return tx.RecordApplied(ctx, 1, "evt-1", 7, domain.PartitionClock{Watermark: testNow}, testNow)
		}, "mark inbox"},
	}
	checkTxFailures(t, cases)
}

func TestStoreReadsNameTheirFailureOnAClosedDatabase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		call func(context.Context, Store) error
		want string
	}{
		{"checkpoint", func(ctx context.Context, s Store) error { _, err := s.LoadCheckpoint(ctx, 1); return err }, "query checkpoint"},
		{"applied position", func(ctx context.Context, s Store) error { _, err := s.AppliedThrough(ctx); return err }, "query applied position"},
		{"timer partitions", func(ctx context.Context, s Store) error { _, err := s.DueTimerPartitions(ctx, testNow); return err }, "query timer partitions"},
		{"current situations", func(ctx context.Context, s Store) error {
			return s.EachCurrentSituation(ctx, func(domain.StoredSituation) error { return nil })
		}, "query current situations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := openStore(t)
			if err := s.db.Close(); err != nil {
				t.Fatal(err)
			}
			if err := tc.call(t.Context(), s); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
