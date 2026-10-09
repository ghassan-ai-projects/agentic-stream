package app

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestKilledEpochStaysTerminalAndRefusesDecisionsAndAdmission(t *testing.T) {
	t.Parallel()
	epochs := epochsAt(openStore(t), sources.NewVirtual(base))
	if err := Drain(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	if err := AssertAdmission(t.Context(), epochs, "e"); !errors.Is(err, domain.ErrEpochDraining) {
		t.Fatalf("draining admission = %v", err)
	}
	if err := Kill(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	if err := Drain(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	if state, err := State(t.Context(), epochs, "e"); err != nil || state != domain.EpochKilled {
		t.Fatalf("state = %q err=%v", state, err)
	}
	if err := AssertDecision(t.Context(), epochs, "e"); !errors.Is(err, domain.ErrEpochKilled) {
		t.Fatalf("decision = %v", err)
	}
	if err := AssertDecision(t.Context(), epochs, ""); !errors.Is(err, domain.ErrEpochUnbound) {
		t.Fatalf("unbound decision = %v", err)
	}
}

func TestEpochGatesOnATransaction(t *testing.T) {
	t.Parallel()
	persistence := openStore(t)
	epochs := epochsAt(persistence, sources.NewVirtual(base))
	if err := Drain(t.Context(), epochs, "e"); err != nil {
		t.Fatal(err)
	}
	err := persistence.WithTx(t.Context(), func(tx *store.Tx) error {
		if err := AssertOrdinaryTx(t.Context(), epochs, tx, "e"); !errors.Is(err, domain.ErrEpochDraining) {
			t.Errorf("ordinary on draining = %v", err)
		}
		if err := AssertOrdinaryTx(t.Context(), epochs, tx, "other"); err != nil {
			t.Errorf("ordinary on an uncontrolled epoch = %v", err)
		}
		if err := AssertDecisionTx(t.Context(), epochs, tx, "e"); err != nil {
			t.Errorf("decision on draining = %v", err)
		}
		if err := AssertDecisionTx(t.Context(), epochs, tx, ""); !errors.Is(err, domain.ErrEpochUnbound) {
			t.Errorf("unbound decision = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
