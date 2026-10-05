package store

import (
	"context"
	"database/sql"
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

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	return db
}

// inTx runs fn in a committed transaction and fails the test on error.
func inTx(t *testing.T, db *storage.DB, fn func(*sql.Tx) error) {
	t.Helper()
	if err := db.WithTx(t.Context(), fn); err != nil {
		t.Fatal(err)
	}
}

func TestClaimsRoundTrip(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	if held, err := LoadClaim(t.Context(), db, claim.Target); err != nil || held != nil {
		t.Fatalf("unclaimed target = %+v, %v", held, err)
	}
	lease := testNow.Add(time.Minute)
	inTx(t, db, func(tx *sql.Tx) error { return WriteClaim(t.Context(), tx, claim, 3, lease, testNow) })
	held, err := LoadClaim(t.Context(), db, claim.Target)
	if err != nil || held.TargetClaim != claim || held.Fence != 3 || !held.LeaseUntil.Equal(lease) || held.Status != domain.ClaimActive {
		t.Fatalf("held claim = %+v, %v", held, err)
	}
	inTx(t, db, func(tx *sql.Tx) error { return MarkClaimReleased(t.Context(), tx, claim.Target, testNow) })
	if held, err = LoadClaim(t.Context(), db, claim.Target); err != nil || held.Status != domain.ClaimReleased {
		t.Fatalf("released claim = %+v, %v", held, err)
	}
}

func TestBindingsRoundTrip(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	digest := "sha256:abababababababababababababababababababababababababababababababab"
	withDigest := domain.CommandBinding{CommandID: "cmd-1", Target: "fan-01", Device: bootA, Owner: owner, CommandDigest: digest}
	withoutDigest := withDigest
	withoutDigest.CommandID, withoutDigest.CommandDigest = "cmd-2", ""
	inTx(t, db, func(tx *sql.Tx) error { return InsertBinding(t.Context(), tx, withDigest, testNow) })
	inTx(t, db, func(tx *sql.Tx) error { return InsertBinding(t.Context(), tx, withoutDigest, testNow) })
	for _, want := range []domain.CommandBinding{withDigest, withoutDigest} {
		if got, err := LoadBinding(t.Context(), db, want.CommandID); err != nil || *got != want {
			t.Fatalf("binding = %+v, %v; want %+v", got, err, want)
		}
	}
	if got, err := LoadBinding(t.Context(), db, "unbound"); err != nil || got != nil {
		t.Fatalf("unbound command = %+v, %v", got, err)
	}
	invalid := withDigest
	invalid.CommandID, invalid.CommandDigest = "cmd-3", "bad"
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return InsertBinding(t.Context(), tx, invalid, testNow) }); err == nil {
		t.Fatal("invalid digest was stored")
	}
}

func TestEventsAndSafeStopLatch(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	inTx(t, db, func(tx *sql.Tx) error {
		return AppendAuthorityEvent(t.Context(), tx, domain.ReleaseEvent(claim, testNow))
	})
	if latched, err := SafeStopLatched(t.Context(), db, bootA); err != nil || latched {
		t.Fatalf("latched without a safe stop = %v, %v", latched, err)
	}
	inTx(t, db, func(tx *sql.Tx) error {
		return AppendAuthorityEvent(t.Context(), tx, domain.SafeStopEvent(claim, domain.SafeStopFailed, nil, testNow))
	})
	if latched, err := SafeStopLatched(t.Context(), db, bootA); err != nil || !latched {
		t.Fatalf("safe stop did not latch = %v, %v", latched, err)
	}
	otherBoot := domain.DeviceBoot{DeviceID: bootA.DeviceID, BootID: "boot-B"}
	if latched, err := SafeStopLatched(t.Context(), db, otherBoot); err != nil || latched {
		t.Fatalf("latch leaked to a new boot = %v, %v", latched, err)
	}
}

func TestSafetyEventsStoreOptionalCommand(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	for _, commandID := range []string{"", "cmd-1"} {
		event := domain.SafetyEvent{Type: domain.SafetyUnsafeOutput, Target: "fan-01", CommandID: commandID, Details: map[string]any{}, Occurred: testNow}
		inTx(t, db, func(tx *sql.Tx) error { return AppendSafetyEvent(t.Context(), tx, event) })
	}
	var withCommand, total int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(command_id), COUNT(*) FROM device_safety_events`).Scan(&withCommand, &total); err != nil {
		t.Fatal(err)
	}
	if withCommand != 1 || total != 2 {
		t.Fatalf("safety events with command=%d total=%d", withCommand, total)
	}
}
