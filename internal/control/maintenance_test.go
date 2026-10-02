package control_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

func TestEpochControlStateMachine(t *testing.T) {
	db, now := openOwnerDB(t)
	ctx := t.Context()
	control := &runtimecontrol.EpochControl{DB: db, Now: func() time.Time { return now }}

	ordinary := func(epoch string) error {
		return db.WithTx(ctx, func(tx *sql.Tx) error { return control.AssertOrdinaryTx(ctx, tx, epoch) })
	}

	if err := control.AssertAdmission(ctx, "epoch-a"); err != nil {
		t.Fatalf("uncontrolled admission: %v", err)
	}
	if err := ordinary("epoch-a"); err != nil {
		t.Fatalf("uncontrolled ordinary work: %v", err)
	}
	if err := control.AssertDecision(ctx, ""); !errors.Is(err, runtimecontrol.ErrEpochUnbound) {
		t.Fatalf("unbound decision = %v", err)
	}

	if err := control.Drain(ctx, "epoch-a"); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if err := control.AssertAdmission(ctx, "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochDraining) {
		t.Fatalf("draining admission = %v", err)
	}
	if err := ordinary("epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochDraining) {
		t.Fatalf("draining ordinary work = %v", err)
	}
	if err := control.AssertDecision(ctx, "epoch-a"); err != nil {
		t.Fatalf("in-flight decision while draining must finish: %v", err)
	}

	if err := control.Kill(ctx, "epoch-a"); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if err := control.Drain(ctx, "epoch-a"); err != nil {
		t.Fatalf("drain after kill: %v", err)
	}
	if state, err := control.State(ctx, "epoch-a"); err != nil || state != "killed" {
		t.Fatalf("kill must be terminal: state=%q err=%v", state, err)
	}
	if err := control.AssertAdmission(ctx, "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochDraining) {
		t.Fatalf("killed admission = %v", err)
	}
	if err := ordinary("epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("killed ordinary work = %v", err)
	}
	if err := control.AssertDecision(ctx, "epoch-a"); !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("killed decision = %v", err)
	}

	var unconfigured *runtimecontrol.EpochControl
	if err := unconfigured.AssertDecision(ctx, "epoch-a"); err == nil {
		t.Fatal("nil epoch control asserted a decision")
	}
	if err := ordinary(""); err == nil {
		t.Fatal("ordinary work without an epoch was allowed")
	}
}
