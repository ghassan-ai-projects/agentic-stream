package storage_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestRuntimeRecoveryRechecksLeaseBeforeCommitting(t *testing.T) {
	t.Parallel()
	db, now := openOwnerDB(t)
	owner := &storage.RuntimeOwner{DB: db, InstanceID: "instance", Lease: time.Minute, Now: func() time.Time { return now }}
	err := owner.ClaimAndRecover(t.Context(), "epoch", func(tx *sql.Tx, acquiredAt time.Time) error {
		if !acquiredAt.Equal(now) {
			t.Fatalf("recovery time = %v, want %v", acquiredAt, now)
		}
		_, err := tx.ExecContext(t.Context(), `INSERT INTO epoch_control VALUES ('recovery-write', 'draining', 'now')`)
		now = now.Add(time.Minute)
		return err
	})
	if !errors.Is(err, storage.ErrRuntimeOwnerBusy) {
		t.Fatalf("expired recovery = %v", err)
	}
	for _, table := range []string{"runtime_owner", "epoch_control"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rollback %s: count=%d err=%v", table, count, err)
		}
	}
}

func TestTargetReleaseRollsBackWhenAuditCannotBeWritten(t *testing.T) {
	t.Parallel()
	db, now := openOwnerDB(t)
	authority := &storage.TargetAuthority{DB: db, InstanceID: "instance", Now: func() time.Time { return now }}
	claim := storage.TargetClaim{Target: "fan", DeviceID: "device", BootID: "boot", AuthorityEpoch: "epoch", OwnerInstance: "instance"}
	if err := authority.Claim(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER reject_release_audit
		BEFORE INSERT ON device_authority_events WHEN NEW.event_type = 'claim_released'
		BEGIN SELECT RAISE(ABORT, 'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := authority.Release(t.Context(), claim); err == nil || !strings.Contains(err.Error(), "record authority event") {
		t.Fatalf("release audit failure = %v", err)
	}
	if err := authority.Assert(t.Context(), claim); err != nil {
		t.Fatalf("claim was released without its audit: %v", err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM device_authority_events WHERE event_type = 'claim_released'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("release audit: count=%d err=%v", count, err)
	}
}

func TestTargetAssertionPreservesIdentityAndExpiryFences(t *testing.T) {
	t.Parallel()
	db, now := openOwnerDB(t)
	authority := &storage.TargetAuthority{DB: db, Now: func() time.Time { return now }, Lease: time.Minute}
	claim := storage.TargetClaim{Target: "fan", DeviceID: "device", BootID: "boot", AuthorityEpoch: "epoch", OwnerInstance: "instance"}
	if err := authority.Claim(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*storage.TargetClaim){
		func(c *storage.TargetClaim) { c.DeviceID = "other" },
		func(c *storage.TargetClaim) { c.BootID = "other" },
		func(c *storage.TargetClaim) { c.AuthorityEpoch = "other" },
		func(c *storage.TargetClaim) { c.OwnerInstance = "other" },
	} {
		other := claim
		mutate(&other)
		if err := authority.Assert(t.Context(), other); !errors.Is(err, storage.ErrTargetClaimNotOwned) {
			t.Fatalf("assert replacement identity %+v = %v", other, err)
		}
		if err := authority.Release(t.Context(), other); !errors.Is(err, storage.ErrTargetClaimNotOwned) {
			t.Fatalf("release replacement identity %+v = %v", other, err)
		}
	}
	now = now.Add(time.Minute)
	if err := authority.Assert(t.Context(), claim); !errors.Is(err, storage.ErrTargetClaimNotOwned) {
		t.Fatalf("assert at expiry = %v", err)
	}
}
