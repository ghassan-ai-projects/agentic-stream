package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestEpochControlStateMachine(t *testing.T) {
	db, now := openOwnerDB(t)
	ctx := t.Context()
	control := &storage.EpochControl{DB: db, Now: func() time.Time { return now }}

	ordinary := func(epoch string) error {
		return db.WithTx(ctx, func(tx *sql.Tx) error { return control.AssertOrdinaryTx(ctx, tx, epoch) })
	}

	if err := control.AssertAdmission(ctx, "epoch-a"); err != nil {
		t.Fatalf("uncontrolled admission: %v", err)
	}
	if err := ordinary("epoch-a"); err != nil {
		t.Fatalf("uncontrolled ordinary work: %v", err)
	}
	if err := control.AssertDecision(ctx, ""); !errors.Is(err, storage.ErrEpochUnbound) {
		t.Fatalf("unbound decision = %v", err)
	}

	if err := control.Drain(ctx, "epoch-a"); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if err := control.AssertAdmission(ctx, "epoch-a"); !errors.Is(err, storage.ErrEpochDraining) {
		t.Fatalf("draining admission = %v", err)
	}
	if err := ordinary("epoch-a"); !errors.Is(err, storage.ErrEpochDraining) {
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
	if err := control.AssertAdmission(ctx, "epoch-a"); !errors.Is(err, storage.ErrEpochDraining) {
		t.Fatalf("killed admission = %v", err)
	}
	if err := ordinary("epoch-a"); !errors.Is(err, storage.ErrEpochKilled) {
		t.Fatalf("killed ordinary work = %v", err)
	}
	if err := control.AssertDecision(ctx, "epoch-a"); !errors.Is(err, storage.ErrEpochKilled) {
		t.Fatalf("killed decision = %v", err)
	}

	var unconfigured *storage.EpochControl
	if err := unconfigured.AssertDecision(ctx, "epoch-a"); err == nil {
		t.Fatal("nil epoch control asserted a decision")
	}
	if err := ordinary(""); err == nil {
		t.Fatal("ordinary work without an epoch was allowed")
	}
}

func TestCalibrationActivationReplacesPriorArtifact(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	store := &storage.CalibrationStore{DB: db}
	first := storage.CalibrationArtifact{Domain: "motors", ModelRevision: "rev-1", ProfileDigest: "p", PromptSHA256: "s", DiagnosisCatalogSHA: "d", PolicyDigest: "pol"}
	second := first
	second.ModelRevision = "rev-2"

	assert := func(artifact storage.CalibrationArtifact) error {
		return db.WithTx(ctx, func(tx *sql.Tx) error { return store.AssertCalibration(ctx, tx, artifact) })
	}
	if err := assert(first); !errors.Is(err, storage.ErrCalibrationMissing) {
		t.Fatalf("missing calibration = %v", err)
	}
	if err := store.Activate(ctx, first, "sha256-first-artifact-digest"); err != nil {
		t.Fatalf("activate first: %v", err)
	}
	if err := assert(first); err != nil {
		t.Fatalf("active calibration rejected: %v", err)
	}
	if err := store.Activate(ctx, second, "sha256-second-artifact-digest"); err != nil {
		t.Fatalf("activate second: %v", err)
	}
	if err := assert(first); !errors.Is(err, storage.ErrCalibrationMissing) {
		t.Fatalf("superseded calibration = %v, want ErrCalibrationMissing", err)
	}
	if err := assert(second); err != nil {
		t.Fatalf("replacement calibration rejected: %v", err)
	}
	if err := (&storage.CalibrationStore{}).Activate(ctx, first, "x"); err == nil {
		t.Fatal("unconfigured store activated an artifact")
	}
}

func TestOpenFreshReservesPathUntilClose(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "replay.db")

	db, err := storage.OpenFresh(ctx, path)
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	if _, err := storage.Open(ctx, path); err == nil {
		t.Fatal("Open succeeded on a path reserved by an active replay")
	}
	if _, err := storage.OpenFresh(ctx, path); err == nil {
		t.Fatal("a second fresh open reused a reserved path")
	}
	if err := db.Checkpoint(ctx); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("reservation not released: %v", err)
	}
	if _, err := storage.OpenFresh(ctx, path); err == nil {
		t.Fatal("OpenFresh accepted an existing database file")
	}
	if _, err := os.Stat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("failed fresh open leaked its reservation: %v", err)
	}
}

func TestRetrySQLiteBusyPassesThroughOtherOutcomes(t *testing.T) {
	t.Parallel()

	calls := 0
	if err := storage.RetrySQLiteBusy(t.Context(), func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("success: err=%v calls=%d", err, calls)
	}
	permanent := errors.New("constraint failed")
	calls = 0
	if err := storage.RetrySQLiteBusy(t.Context(), func() error { calls++; return permanent }); !errors.Is(err, permanent) || calls != 1 {
		t.Fatalf("non-busy errors must not retry: err=%v calls=%d", err, calls)
	}
	if storage.IsSQLiteBusy(permanent) || storage.IsSQLiteBusy(nil) {
		t.Fatal("non-SQLite error classified as busy")
	}
}

func TestRetrySQLiteBusyRetriesRealContention(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "busy.db")
	db, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	holder, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(ctx, `INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('held', 'draining', 'now')`); err != nil {
		t.Fatal(err)
	}

	contender, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = contender.Close() })

	attempts := 0
	released := false
	err = storage.RetrySQLiteBusy(ctx, func() error {
		attempts++
		if attempts == 2 && !released {
			released = true
			if err := holder.Commit(); err != nil {
				t.Errorf("release writer lock: %v", err)
			}
		}
		tx, err := contender.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		return tx.Rollback()
	})
	if err != nil {
		t.Fatalf("retry did not recover from contention: %v", err)
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d; the first attempt should have hit a busy writer lock", attempts)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	busy := 0
	if err := storage.RetrySQLiteBusy(canceledCtx, func() error {
		busy++
		lock, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = lock.Rollback() }()
		if _, err := lock.ExecContext(ctx, `INSERT INTO epoch_control (epoch, state, updated_at) VALUES ('held-2', 'draining', 'now')`); err != nil {
			return err
		}
		tx, err := contender.BeginTx(ctx, nil)
		if err == nil {
			_ = tx.Rollback()
		}
		return err
	}); !errors.Is(err, context.Canceled) || busy != 1 {
		t.Fatalf("canceled retry: err=%v attempts=%d", err, busy)
	}
}
