package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
)

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
		if err := tx.WriteClaim(t.Context(), claim, domain.ClaimDecision{Fence: 1, LeaseUntil: testNow.Add(time.Minute)}, testNow); err != nil {
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
