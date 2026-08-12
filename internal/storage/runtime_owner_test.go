package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestRuntimeOwnerClaimHeartbeatAndRelease(t *testing.T) {
	db, now := openOwnerDB(t)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute, Now: func() time.Time { return now }}

	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := owner.Renew(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := owner.Release(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := owner.Claim(t.Context(), "epoch-2"); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
}

func TestRuntimeOwnerRejectsUnexpiredSecondEpoch(t *testing.T) {
	db, now := openOwnerDB(t)
	first := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute, Now: func() time.Time { return now }}
	second := &storage.RuntimeOwner{DB: db, InstanceID: "instance-2", Lease: time.Minute, Now: func() time.Time { return now }}
	if err := first.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := second.Claim(t.Context(), "epoch-2"); !errors.Is(err, storage.ErrRuntimeOwnerBusy) {
		t.Fatalf("second claim error = %v, want owner busy", err)
	}
}

func TestRuntimeOwnerExpiredLeaseCanBeReplacedAndOldEpochCannotRenew(t *testing.T) {
	db, now := openOwnerDB(t)
	first := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute, Now: func() time.Time { return now }}
	second := &storage.RuntimeOwner{DB: db, InstanceID: "instance-2", Lease: time.Minute, Now: func() time.Time { return now }}
	if err := first.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := second.Claim(t.Context(), "epoch-2"); err != nil {
		t.Fatalf("replacement claim: %v", err)
	}
	if err := first.Renew(t.Context(), "epoch-1"); !errors.Is(err, storage.ErrRuntimeOwnerBusy) {
		t.Fatalf("old renew error = %v, want owner busy", err)
	}
}

func TestRuntimeOwnerAssertRequiresCurrentUnexpiredEpoch(t *testing.T) {
	db, now := openOwnerDB(t)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance-1", Lease: time.Minute, Now: func() time.Time { return now }}
	if err := owner.Claim(t.Context(), "epoch-1"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return owner.Assert(t.Context(), tx, "epoch-1")
	}); err != nil {
		t.Fatalf("assert current owner: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return owner.Assert(t.Context(), tx, "epoch-1")
	}); !errors.Is(err, storage.ErrRuntimeOwnerBusy) {
		t.Fatalf("assert expired owner error = %v, want owner busy", err)
	}
}

func openOwnerDB(t *testing.T) (*storage.DB, time.Time) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	return db, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
}
