package store

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
)

func heartbeatTimer(id, stateKey string, due time.Time) domain.HeartbeatTimer {
	return domain.HeartbeatTimer{ID: id, OperatorID: "hb", StateKey: stateKey, DueAt: due, Payload: []byte(`{"expected_event_id":"evt-1"}`)}
}

func armTimers(t *testing.T, s Store, partition int, timers ...domain.HeartbeatTimer) {
	t.Helper()
	inTx(t, s, func(tx *Tx) error {
		for _, timer := range timers {
			if err := tx.ArmHeartbeatTimer(t.Context(), partition, timer, testNow); err != nil {
				return err
			}
		}
		return nil
	})
}

func dueTimerIDs(t *testing.T, s Store, partition int, now time.Time) []string {
	t.Helper()
	var ids []string
	inTx(t, s, func(tx *Tx) error {
		timers, err := tx.LoadDueTimers(t.Context(), partition, now)
		for _, timer := range timers {
			ids = append(ids, timer.ID)
		}
		return err
	})
	return ids
}

func TestHeartbeatTimersAreReplacedAndAcknowledgedOnce(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	due := testNow.Add(time.Minute)
	timer := heartbeatTimer("tmr-1", "m1", due)
	armTimers(t, s, 0, timer, timer)
	if partitions, err := s.TimerPartitions(t.Context()); err != nil || !slices.Equal(partitions, []int{0}) {
		t.Fatalf("partitions = %v err=%v, want [0]", partitions, err)
	}
	if ids := dueTimerIDs(t, s, 0, testNow); len(ids) != 0 {
		t.Fatalf("a timer before its due time loaded: %v", ids)
	}
	inTx(t, s, func(tx *Tx) error {
		timers, err := tx.LoadDueTimers(t.Context(), 0, due)
		if err != nil || len(timers) != 1 || timers[0].ExpectedEventID != "evt-1" || timers[0].StateKey != "m1" || timers[0].OperatorID != "hb" {
			t.Fatalf("due timers = %+v err=%v, want tmr-1 expecting evt-1", timers, err)
		}
		return tx.AcknowledgeTimers(t.Context(), timers, due)
	})
	if ids := dueTimerIDs(t, s, 0, due); len(ids) != 0 {
		t.Fatalf("a fired timer loaded again: %v", ids)
	}
	armTimers(t, s, 0, timer)
	if ids := dueTimerIDs(t, s, 0, due); len(ids) != 0 {
		t.Fatalf("re-arming revived a fired timer: %v", ids)
	}
	if partitions, err := s.TimerPartitions(t.Context()); err != nil || len(partitions) != 0 {
		t.Fatalf("partitions = %v err=%v, want none once every timer fired", partitions, err)
	}
}

func TestArmingANewHeartbeatTimerCancelsThePendingOneOfTheSameStateKey(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	armTimers(t, s, 0, heartbeatTimer("tmr-early", "m1", testNow.Add(time.Minute)))
	armTimers(t, s, 0, heartbeatTimer("tmr-late", "m1", testNow.Add(5*time.Minute)), heartbeatTimer("tmr-other-key", "m2", testNow.Add(time.Minute)))
	got := dueTimerIDs(t, s, 0, testNow.Add(10*time.Minute))
	slices.Sort(got)
	if !slices.Equal(got, []string{"tmr-late", "tmr-other-key"}) {
		t.Fatalf("due timers = %v, want only the replacing timer and the other state key's", got)
	}
	const wantStatus = "cancelled" //nolint:misspell // the timers table stores this spelling
	var status string
	if err := s.db.QueryRowContext(t.Context(), "SELECT status FROM timers WHERE timer_id = 'tmr-early'").Scan(&status); err != nil || status != wantStatus {
		t.Fatalf("replaced timer status = %q err=%v, want %s", status, err, wantStatus)
	}
}

func TestDueTimersAreOrderedByDueTimeAndScopedToPartitionAndTenant(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	armTimers(t, s, 0, heartbeatTimer("tmr-b", "m2", testNow.Add(2*time.Minute)), heartbeatTimer("tmr-a", "m1", testNow.Add(time.Minute)))
	armTimers(t, s, 3, heartbeatTimer("tmr-elsewhere", "m1", testNow))
	if got := dueTimerIDs(t, s, 0, testNow.Add(time.Hour)); !slices.Equal(got, []string{"tmr-a", "tmr-b"}) {
		t.Fatalf("due timers = %v, want due-time order", got)
	}
	if partitions, err := s.TimerPartitions(t.Context()); err != nil || !slices.Equal(partitions, []int{0, 3}) {
		t.Fatalf("partitions = %v err=%v, want [0 3]", partitions, err)
	}
	other := New(s.db, allowOwner, "epoch", "other", s.deploymentID)
	if partitions, err := other.TimerPartitions(t.Context()); err != nil || len(partitions) != 0 {
		t.Fatalf("another tenant's partitions = %v err=%v, want none", partitions, err)
	}
}

func TestCorruptTimerPayloadRefusesTheBatch(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	bad := heartbeatTimer("tmr-bad", "m1", testNow)
	bad.Payload = []byte("not json")
	armTimers(t, s, 0, bad)
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		_, err := tx.LoadDueTimers(t.Context(), 0, testNow)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "decode timer payload tmr-bad") {
		t.Fatalf("err = %v, want decode timer payload tmr-bad", err)
	}
}

func TestTimerStatementsNameTheirFailure(t *testing.T) {
	t.Parallel()
	timer := heartbeatTimer("tmr-1", "m1", testNow)
	cases := []txFailure{
		{"arm", "timers", func(ctx context.Context, tx *Tx) error { return tx.ArmHeartbeatTimer(ctx, 0, timer, testNow) }, "cancel prior heartbeat timer"},
		{"load", "timers", func(ctx context.Context, tx *Tx) error { _, err := tx.LoadDueTimers(ctx, 0, testNow); return err }, "query due timers"},
		{"acknowledge", "timers", func(ctx context.Context, tx *Tx) error {
			return tx.AcknowledgeTimers(ctx, []domain.DueTimer{{ID: "tmr-1"}}, testNow)
		}, "acknowledge timers"},
	}
	checkTxFailures(t, cases)
}
