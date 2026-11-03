package store

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAnOpenCircuitIsListedClosedOnceAndReopenedByItsNextCause(t *testing.T) {
	t.Parallel()
	s := New(storagetest.OpenTemp(t))
	rule := domain.RuleKey{ID: "bearing-wear", Version: 1}
	inTx(t, s, func(tx *Tx) error {
		if err := tx.OpenCircuit(t.Context(), domain.Circuit{Rule: rule, TenantID: "t", Cause: domain.CauseRateCap, OpenedAt: storeNow}); err != nil {
			t.Fatal(err)
		}
		if err := tx.CloseCircuit(t.Context(), "t", rule, "reviewed", storeNow); err != nil {
			t.Fatal(err)
		}
		if err := tx.CloseCircuit(t.Context(), "t", rule, "again", storeNow); !errors.Is(err, domain.ErrNoOpenCircuit) {
			t.Fatalf("closing a closed circuit = %v, want no open circuit", err)
		}
		circuits, err := tx.Circuits(t.Context(), "t")
		if err != nil || len(circuits) != 1 || circuits[0].State != "closed" || circuits[0].CloseReason != "reviewed" {
			t.Fatalf("circuits = %+v, %v; want the closed one with its reason", circuits, err)
		}
		if err := tx.OpenCircuit(t.Context(), domain.Circuit{Rule: rule, TenantID: "t", Cause: domain.CauseKindChanged, OpenedAt: storeNow}); err != nil {
			t.Fatal(err)
		}
		open, err := tx.OpenCircuits(t.Context(), "t")
		if err != nil || !open[rule] {
			t.Fatalf("open circuits = %v, %v; want the rule reopened", open, err)
		}
		return nil
	})
}

func TestTheLatestBornKindDigestIsKept(t *testing.T) {
	t.Parallel()
	s := New(storagetest.OpenTemp(t))
	inTx(t, s, func(tx *Tx) error {
		for _, digest := range []string{"sha256:old", "sha256:new"} {
			if err := tx.RecordKindDigests(t.Context(), "t", map[string]string{"pump": digest, "fan": "sha256:fan"}, storeNow); err != nil {
				t.Fatal(err)
			}
		}
		digests, err := tx.KindDigests(t.Context(), "t")
		if err != nil || digests["pump"] != "sha256:new" || digests["fan"] != "sha256:fan" || len(digests) != 2 {
			t.Fatalf("kind digests = %v, %v; want the latest of each kind", digests, err)
		}
		return nil
	})
}
