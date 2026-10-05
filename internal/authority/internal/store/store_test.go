package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var (
	testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	owner   = domain.Owner{Epoch: "epoch-1", Instance: "instance-1"}
	bootA   = domain.DeviceBoot{DeviceID: "thermal-01", BootID: "boot-A"}
	claim   = domain.TargetClaim{Target: "fan-01", Device: bootA, Owner: owner}
)

func openStore(t *testing.T) (*Store, *storage.DB) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	return New(db), db
}

// work runs fn in a committed priority unit of work and fails the test on error.
func work(t *testing.T, s *Store, fn func(*Tx) error) {
	t.Helper()
	if err := s.InTx(t.Context(), fn); err != nil {
		t.Fatal(err)
	}
}

func TestUnitOfWorkRunsFencesOnItsTransaction(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	refused := errors.New("fence refused")
	var seenEpoch string
	fence := func(_ context.Context, tx *sql.Tx, epoch string) error {
		seenEpoch = epoch
		if tx == nil {
			return errors.New("fence ran outside the transaction")
		}
		return refused
	}
	err := s.InTx(t.Context(), func(tx *Tx) error { return tx.Assert(t.Context(), fence, "epoch-1") })
	if !errors.Is(err, refused) || seenEpoch != "epoch-1" {
		t.Fatalf("fence err=%v epoch=%q", err, seenEpoch)
	}
}

func TestUnitOfWorkRollsBackTogether(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	failed := errors.New("audit failed")
	err := s.InTx(t.Context(), func(tx *Tx) error {
		if err := tx.WriteClaim(t.Context(), claim, 1, testNow.Add(time.Minute), testNow); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("unit of work = %v", err)
	}
	work(t, s, func(tx *Tx) error {
		held, err := tx.LoadClaim(t.Context(), claim.Target)
		if err != nil || held != nil {
			t.Fatalf("claim survived rollback: %+v, %v", held, err)
		}
		return nil
	})
}

func TestClaimsRoundTrip(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	lease := testNow.Add(time.Minute)
	work(t, s, func(tx *Tx) error { return tx.WriteClaim(t.Context(), claim, 3, lease, testNow) })
	work(t, s, func(tx *Tx) error {
		held, err := tx.LoadClaim(t.Context(), claim.Target)
		if err != nil || held.TargetClaim != claim || held.Fence != 3 || !held.LeaseUntil.Equal(lease) || held.Status != domain.ClaimActive {
			t.Fatalf("held claim = %+v, %v", held, err)
		}
		return tx.MarkClaimReleased(t.Context(), claim.Target, testNow)
	})
	work(t, s, func(tx *Tx) error {
		if held, err := tx.LoadClaim(t.Context(), claim.Target); err != nil || held.Status != domain.ClaimReleased {
			t.Fatalf("released claim = %+v, %v", held, err)
		}
		return nil
	})
}

func TestBindingsRoundTrip(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	digest := "sha256:abababababababababababababababababababababababababababababababab"
	withDigest := domain.CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootA, Owner: owner, CommandDigest: digest}
	withoutDigest := withDigest
	withoutDigest.CommandID, withoutDigest.CommandDigest = "cmd-2", ""
	work(t, s, func(tx *Tx) error { return tx.InsertBinding(t.Context(), withDigest, testNow) })
	work(t, s, func(tx *Tx) error { return tx.InsertBinding(t.Context(), withoutDigest, testNow) })
	inCallerTx := func(fn func(*Tx) error) {
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return fn(Join(tx)) }); err != nil {
			t.Fatal(err)
		}
	}
	inCallerTx(func(tx *Tx) error {
		for _, want := range []domain.CommandBinding{withDigest, withoutDigest} {
			if got, err := tx.LoadBinding(t.Context(), want.CommandID); err != nil || *got != want {
				t.Fatalf("binding = %+v, %v; want %+v", got, err, want)
			}
		}
		if got, err := tx.LoadBinding(t.Context(), "unbound"); err != nil || got != nil {
			t.Fatalf("unbound command = %+v, %v", got, err)
		}
		return nil
	})
	invalid := withDigest
	invalid.CommandID, invalid.CommandDigest = "cmd-3", "bad"
	if err := s.InTx(t.Context(), func(tx *Tx) error { return tx.InsertBinding(t.Context(), invalid, testNow) }); err == nil {
		t.Fatal("invalid digest was stored")
	}
}

func TestEventsAndSafeStopLatch(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	work(t, s, func(tx *Tx) error { return tx.AppendAuthorityEvent(t.Context(), domain.ReleaseEvent(claim, testNow)) })
	if latched, err := s.SafeStopLatched(t.Context(), bootA); err != nil || latched {
		t.Fatalf("latched without a safe stop = %v, %v", latched, err)
	}
	work(t, s, func(tx *Tx) error {
		return tx.AppendAuthorityEvent(t.Context(), domain.SafeStopEvent(claim, domain.SafeStopFailed, nil, testNow))
	})
	if latched, err := s.SafeStopLatched(t.Context(), bootA); err != nil || !latched {
		t.Fatalf("safe stop did not latch = %v, %v", latched, err)
	}
	otherBoot := domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-B"}
	if latched, err := s.SafeStopLatched(t.Context(), otherBoot); err != nil || latched {
		t.Fatalf("latch leaked to a new boot = %v, %v", latched, err)
	}
}

func TestSafetyEventsStoreOptionalCommand(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	for _, commandID := range []string{"", "cmd-1"} {
		event := domain.SafetyEvent{Type: domain.SafetyUnsafeOutput, Target: "fan-01", CommandID: commandID, Details: map[string]any{}, Occurred: testNow}
		work(t, s, func(tx *Tx) error { return tx.AppendSafetyEvent(t.Context(), event) })
	}
	var withCommand, total int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(command_id), COUNT(*) FROM device_safety_events`).Scan(&withCommand, &total); err != nil {
		t.Fatal(err)
	}
	if withCommand != 1 || total != 2 {
		t.Fatalf("safety events with command=%d total=%d", withCommand, total)
	}
}

func TestSafetyRecordReadsVerifyStoredEvidence(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	event := domain.SafetyEvent{Type: domain.SafetyPhysicalTransition, Target: "fan-01", Details: map[string]any{"source": "s"}, Occurred: testNow}
	work(t, s, func(tx *Tx) error { return tx.AppendSafetyEvent(t.Context(), event) })
	work(t, s, func(tx *Tx) error { return tx.InsertFirstState(t.Context(), deviceState(t, bootA), owner, testNow) })
	work(t, s, func(tx *Tx) error {
		events, err := tx.SafetyEvents(t.Context())
		if err != nil || len(events) != 1 || events[0].Details["source"] != "s" || !events[0].Occurred.Equal(testNow) {
			t.Fatalf("events = %+v, %v", events, err)
		}
		open, err := tx.CountOpenReconciliations(t.Context())
		if err != nil || open != 0 {
			t.Fatalf("open reconciliations = %d, %v", open, err)
		}
		return nil
	})
	if _, err := db.ExecContext(t.Context(), `UPDATE device_safety_events SET details_json = CAST('{"source":"x"}' AS BLOB)`); err != nil {
		t.Fatal(err)
	}
	err := s.InTx(t.Context(), func(tx *Tx) error {
		_, err := tx.SafetyEvents(t.Context())
		return err
	})
	if err == nil {
		t.Fatal("tampered safety evidence was read")
	}
}
