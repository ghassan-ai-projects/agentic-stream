package control_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newEpochControl(t *testing.T) (*storage.DB, *runtimecontrol.EpochControl) {
	t.Helper()
	db := openOwnerDB(t)
	return db, &runtimecontrol.EpochControl{DB: db, Now: sources.NewVirtual(epoch0).Now}
}

func TestAnUncontrolledEpochAdmitsAndRunsWork(t *testing.T) {
	t.Parallel()
	db, control := newEpochControl(t)
	if err := control.AssertAdmission(t.Context(), "epoch-a"); err != nil {
		t.Fatalf("uncontrolled admission: %v", err)
	}
	if err := fenced(t, db, control.AssertOrdinaryTx, "epoch-a"); err != nil {
		t.Fatalf("uncontrolled ordinary work: %v", err)
	}
	if err := control.AssertDecision(t.Context(), "epoch-a"); err != nil {
		t.Fatalf("uncontrolled decision: %v", err)
	}
	if state, err := control.State(t.Context(), "epoch-a"); err != nil || state != "" {
		t.Fatalf("uncontrolled state = %q, %v", state, err)
	}
}

func TestADrainingEpochRefusesNewWorkButLetsInFlightDecisionsFinish(t *testing.T) {
	t.Parallel()
	db, control := newEpochControl(t)
	if err := control.Drain(t.Context(), "epoch-a"); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if err := control.AssertAdmission(t.Context(), "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochDraining) {
		t.Fatalf("draining admission = %v, want ErrEpochDraining", err)
	}
	if err := fenced(t, db, control.AssertOrdinaryTx, "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochDraining) {
		t.Fatalf("draining ordinary work = %v, want ErrEpochDraining", err)
	}
	if err := control.AssertDecision(t.Context(), "epoch-a"); err != nil {
		t.Fatalf("an in-flight decision must finish while draining: %v", err)
	}
}

func TestAKilledEpochIsTerminalAndRefusesAdmissionWorkAndDecisions(t *testing.T) {
	t.Parallel()
	db, control := newEpochControl(t)
	if err := control.Kill(t.Context(), "epoch-a"); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if err := control.Drain(t.Context(), "epoch-a"); err != nil {
		t.Fatalf("drain after kill: %v", err)
	}
	if state, err := control.State(t.Context(), "epoch-a"); err != nil || state != "killed" {
		t.Fatalf("a kill must not be downgraded by a drain: state=%q err=%v", state, err)
	}
	if err := control.AssertAdmission(t.Context(), "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochDraining) {
		t.Fatalf("killed admission = %v, want ErrEpochDraining", err)
	}
	if err := fenced(t, db, control.AssertOrdinaryTx, "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("killed ordinary work = %v, want ErrEpochKilled", err)
	}
	if err := control.AssertDecision(t.Context(), "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("killed decision = %v, want ErrEpochKilled", err)
	}
}

func TestAnEpochControlThatCannotNameAnEpochFailsClosed(t *testing.T) {
	t.Parallel()
	db, control := newEpochControl(t)
	var missing *runtimecontrol.EpochControl
	noDatabase := &runtimecontrol.EpochControl{}
	if err := control.AssertDecision(t.Context(), ""); !errors.Is(err, runtimecontrol.ErrEpochUnbound) {
		t.Fatalf("unbound decision = %v, want ErrEpochUnbound", err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return control.AssertDecisionTx(t.Context(), tx, "") }); !errors.Is(err, runtimecontrol.ErrEpochUnbound) {
		t.Fatalf("unbound transactional decision = %v, want ErrEpochUnbound", err)
	}
	assertRefusal(t, fenced(t, db, control.AssertOrdinaryTx, ""), "epoch control is not configured")
	for name, failure := range map[string]error{
		"nil receiver state":              errOf(missing.State(t.Context(), "epoch")),
		"nil receiver decision":           missing.AssertDecision(t.Context(), "epoch"),
		"no database admission":           noDatabase.AssertAdmission(t.Context(), "epoch"),
		"no database kill":                noDatabase.Kill(t.Context(), "epoch"),
		"no database drain":               noDatabase.Drain(t.Context(), "epoch"),
		"empty epoch state":               errOf(control.State(t.Context(), "")),
		"empty epoch kill":                control.Kill(t.Context(), ""),
		"empty epoch drain":               control.Drain(t.Context(), ""),
		"no transaction":                  control.AssertDecisionTx(t.Context(), nil, "epoch"),
		"no database transactional check": noDatabase.AssertDecisionTx(t.Context(), nil, "epoch"),
	} {
		if failure == nil || !strings.Contains(failure.Error(), "epoch control is not configured") {
			t.Errorf("%s = %v, want an epoch-control-not-configured refusal", name, failure)
		}
	}
}

func errOf(_ string, err error) error { return err }
