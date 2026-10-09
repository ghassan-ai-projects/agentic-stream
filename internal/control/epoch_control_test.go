package control_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/controltest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const killedAt = "2026-08-12T12:34:56.123456789Z"

func killClock() time.Time { return time.Date(2026, 8, 12, 12, 34, 56, 123456789, time.UTC) }

func openOrphanTolerantDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	db.SetMaxOpenConns(1)
	return db
}

func episodeLifecycle(t *testing.T, db *storage.DB, episodeID string) (lifecycle string, endedAt sql.NullString) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(), "SELECT lifecycle_status, ended_at FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle, &endedAt); err != nil {
		t.Fatalf("read episode %s: %v", episodeID, err)
	}
	return lifecycle, endedAt
}

func TestKillingAnEpochRecordsItAndSupersedesItsInFlightEpisodesAtomically(t *testing.T) {
	t.Parallel()
	db := openOrphanTolerantDB(t)
	seedEpisode(t, db, "epi-kill-atomic", "epoch-kill-atomic", "running")
	control := &runtimecontrol.EpochControl{DB: db, Now: killClock}
	if err := control.Kill(t.Context(), "epoch-kill-atomic"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}

	var state, updatedAt string
	if err := db.QueryRowContext(t.Context(), "SELECT state, updated_at FROM epoch_control WHERE epoch = ?", "epoch-kill-atomic").Scan(&state, &updatedAt); err != nil {
		t.Fatalf("read epoch control: %v", err)
	}
	lifecycle, endedAt := episodeLifecycle(t, db, "epi-kill-atomic")
	if state != "killed" || updatedAt != killedAt || lifecycle != "superseded" || endedAt.String != killedAt {
		t.Fatalf("state=%q updated_at=%q lifecycle=%q ended_at=%q", state, updatedAt, lifecycle, endedAt.String)
	}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return control.AssertDecisionTx(t.Context(), tx, "epoch-kill-atomic") })
	if !errors.Is(err, runtimecontrol.ErrEpochKilled) {
		t.Fatalf("transactional decision assertion = %v, want ErrEpochKilled", err)
	}
}

func TestKillingAnEpochRollsBackTheRecordWhenSupersessionFails(t *testing.T) {
	t.Parallel()
	db := openOrphanTolerantDB(t)
	seedEpisode(t, db, "epi-kill-rollback", "epoch-kill-rollback", "running")
	if _, err := db.ExecContext(t.Context(), `
		CREATE TRIGGER abort_epoch_supersession
		BEFORE UPDATE OF lifecycle_status ON episodes
		WHEN NEW.lifecycle_status = 'superseded'
		BEGIN
			SELECT RAISE(ABORT, 'forced supersession failure');
		END`); err != nil {
		t.Fatalf("install supersession failure trigger: %v", err)
	}

	err := (&runtimecontrol.EpochControl{DB: db}).Kill(t.Context(), "epoch-kill-rollback")
	assertRefusal(t, err, "forced supersession failure")

	var state string
	if err := db.QueryRowContext(t.Context(), "SELECT state FROM epoch_control WHERE epoch = ?", "epoch-kill-rollback").Scan(&state); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("epoch control after rollback = %q, err=%v; want no row", state, err)
	}
	if lifecycle, _ := episodeLifecycle(t, db, "epi-kill-rollback"); lifecycle != "running" {
		t.Fatalf("episode lifecycle after rollback = %q, want running", lifecycle)
	}
}

func TestKillingAnEpochReleasesTheCostReservationOfAnAdmittedEpisodeThatNeverStarted(t *testing.T) {
	t.Parallel()
	db := openOrphanTolerantDB(t)
	seedEpisode(t, db, "epi-kill-cost", "epoch-kill-cost", "admitted")
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := controltest.SetCostLimit(t.Context(), tx, "global", "", 100, false, "2026-08-12T10:00:00Z"); err != nil {
			return err
		}
		if err := controltest.SetCostLimit(t.Context(), tx, "tenant:tenant", "tenant", 100, false, "2026-08-12T10:00:00Z"); err != nil {
			return err
		}
		return (runtimecontrol.CostLedger{}).Reserve(t.Context(), tx, "epi-kill-cost", "tenant", 10, "2026-08-12T10:00:00Z")
	}); err != nil {
		t.Fatalf("reserve cost for the admitted episode: %v", err)
	}
	assertCostTotals(t, db, 10, 0, 0)

	if err := (&runtimecontrol.EpochControl{DB: db, Now: killClock}).Kill(t.Context(), "epoch-kill-cost"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}
	var status string
	var reserved, actual int64
	if err := db.QueryRowContext(t.Context(), `SELECT status, reserved_micro, actual_micro FROM cost_reservations WHERE episode_id = 'epi-kill-cost'`).Scan(&status, &reserved, &actual); err != nil {
		t.Fatalf("read released reservation: %v", err)
	}
	if status != "settled" || reserved != 10 || actual != 0 {
		t.Fatalf("reservation status=%q reserved=%d actual=%d, want settled/10/0", status, reserved, actual)
	}
	assertCostTotals(t, db, 0, 0, 0)
}
