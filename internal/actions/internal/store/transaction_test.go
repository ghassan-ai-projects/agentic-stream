package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

func TestAStoreIsConfiguredOnlyWithItsDatabaseAndOwnerCheck(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	cases := []struct {
		name  string
		store Store
		want  bool
	}{
		{"complete", newStore(db), true},
		{"without a database", New(nil, allowOwner, "epoch"), false},
		{"without an owner check", New(db, nil, "epoch"), false},
		{"reader that cannot write", Reader(db), false},
	}
	for _, tc := range cases {
		if got := tc.store.Configured(); got != tc.want {
			t.Errorf("%s: Configured = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRuntimeOwnershipIsAssertedOnTheTransactionOrRefused(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	lost := errors.New("owned elsewhere")
	assertOwner := func(s Store) error {
		return s.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertOwner(t.Context()) })
	}
	if err := assertOwner(newStore(db)); err != nil {
		t.Fatalf("a current owner was refused: %v", err)
	}
	if err := assertOwner(New(db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch")); !errors.Is(err, lost) || !strings.Contains(err.Error(), "ownership lost") {
		t.Fatalf("err = %v, want the ownership error wrapped as lost ownership", err)
	}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return JoinCaller(tx).AssertOwner(t.Context()) })
	if err == nil || !strings.Contains(err.Error(), "owner is not configured") {
		t.Fatalf("err = %v, want a caller-joined transaction without an owner check refused", err)
	}
}

func TestATrippedInterlockRefusesTheCommandAndTheDispatchAuthorization(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	s := newStore(db)
	if err := s.DispatchAuthorization().Check(t.Context()); err != nil {
		t.Fatalf("a ready interlock refused the dispatch authorization: %v", err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := interlock.TripIn(t.Context(), tx, "stop", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertInterlock(t.Context()) })
	if err == nil || !strings.Contains(err.Error(), "interlock rejected command") {
		t.Fatalf("AssertInterlock err = %v, want the interlock refusal", err)
	}
	if err := s.DispatchAuthorization().Check(t.Context()); err == nil {
		t.Fatal("the dispatch authorization passed a tripped interlock")
	}
}

func TestAFailedUnitOfWorkRollsBackEveryWrite(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	boom := errors.New("boom")
	err := newStore(db).WithTx(t.Context(), func(tx *Tx) error {
		if err := tx.MarkCommandDispatching(t.Context(), commandID, testNow); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the unit of work's error", err)
	}
	if status := queryString(t, db, "SELECT status FROM commands WHERE command_id = ?", commandID); status != "pending" {
		t.Fatalf("status = %q, want pending: the failed unit of work must roll back", status)
	}
}

func TestACommandHasOneIdempotencyKeyAndNoSecondLedgerEntryForAReplay(t *testing.T) {
	t.Parallel()
	db, commandID := openActionFixture(t)
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO commands (command_id, intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		SELECT 'cmd-replay', intent_id, tenant_id, effector_route, normalized_target, idempotency_key, command_json, command_sha256, 'pending', created_at, updated_at
		FROM commands WHERE command_id = ?`, commandID)
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: commands.idempotency_key") {
		t.Fatalf("err = %v, want the ledger to refuse a second command with the same idempotency key", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM commands"); n != 1 {
		t.Fatalf("commands = %d, want the original only", n)
	}
}
