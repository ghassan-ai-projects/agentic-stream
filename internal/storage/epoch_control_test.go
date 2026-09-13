package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestEpochControlKillIsAtomicWithEpisodeSupersession(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	seedRunningEpochEpisode(t, db, "epi-kill-atomic", "epoch-kill-atomic")

	now := time.Date(2026, 8, 12, 12, 34, 56, 123456789, time.UTC)
	control := &storage.EpochControl{DB: db, Now: func() time.Time { return now }}
	if err := control.Kill(ctx, "epoch-kill-atomic"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}

	var state, updatedAt, lifecycle, endedAt string
	if err := db.QueryRowContext(ctx, "SELECT state, updated_at FROM epoch_control WHERE epoch = ?", "epoch-kill-atomic").Scan(&state, &updatedAt); err != nil {
		t.Fatalf("read epoch control: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT lifecycle_status, ended_at FROM episodes WHERE episode_id = ?", "epi-kill-atomic").Scan(&lifecycle, &endedAt); err != nil {
		t.Fatalf("read superseded episode: %v", err)
	}
	wantTime := "2026-08-12T12:34:56.123456789Z"
	if state != "killed" || updatedAt != wantTime || lifecycle != "superseded" || endedAt != wantTime {
		t.Fatalf("state=%q updated_at=%q lifecycle=%q ended_at=%q", state, updatedAt, lifecycle, endedAt)
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return control.AssertDecisionTx(ctx, tx, "epoch-kill-atomic")
	}); !errors.Is(err, storage.ErrEpochKilled) {
		t.Fatalf("transactional decision assertion = %v, want ErrEpochKilled", err)
	}
	if err := control.Drain(ctx, "epoch-kill-atomic"); err != nil {
		t.Fatalf("drain after kill: %v", err)
	}
	state, err := control.State(ctx, "epoch-kill-atomic")
	if err != nil {
		t.Fatalf("read terminal epoch state: %v", err)
	}
	if state != "killed" {
		t.Fatalf("epoch state after drain = %q, want killed", state)
	}
}

func TestEpochControlKillRollsBackWhenSupersessionFails(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	seedRunningEpochEpisode(t, db, "epi-kill-rollback", "epoch-kill-rollback")
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER abort_epoch_supersession
		BEFORE UPDATE OF lifecycle_status ON episodes
		WHEN NEW.lifecycle_status = 'superseded'
		BEGIN
			SELECT RAISE(ABORT, 'forced supersession failure');
		END`); err != nil {
		t.Fatalf("install supersession failure trigger: %v", err)
	}

	control := &storage.EpochControl{DB: db}
	if err := control.Kill(ctx, "epoch-kill-rollback"); err == nil {
		t.Fatal("Kill succeeded despite supersession failure")
	}

	var state string
	if err := db.QueryRowContext(ctx, "SELECT state FROM epoch_control WHERE epoch = ?", "epoch-kill-rollback").Scan(&state); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("epoch control after rollback = %q, err=%v; want no row", state, err)
	}
	var lifecycle string
	if err := db.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", "epi-kill-rollback").Scan(&lifecycle); err != nil {
		t.Fatalf("read episode after rollback: %v", err)
	}
	if lifecycle != "running" {
		t.Fatalf("episode lifecycle after rollback = %q, want running", lifecycle)
	}
}

func TestEpochControlKillReleasesUnstartedEpisodeReservation(t *testing.T) {
	db, _ := openOwnerDB(t)
	ctx := t.Context()
	seedRunningEpochEpisode(t, db, "epi-kill-cost", "epoch-kill-cost")
	if _, err := db.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'admitted', current_attempt_id = NULL
		WHERE episode_id = 'epi-kill-cost'`); err != nil {
		t.Fatalf("make episode admitted: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO cost_reservations (reservation_id, episode_id, tenant_id, reserved_micro, status, created_at)
		VALUES ('res-epi-kill-cost', 'epi-kill-cost', 'tenant', 10, 'reserved', '2026-08-12T10:00:00Z')`); err != nil {
		t.Fatalf("seed cost reservation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE cost_limits SET reserved_micro = 10 WHERE scope_key = 'global'"); err != nil {
		t.Fatalf("seed global reservation: %v", err)
	}

	control := &storage.EpochControl{DB: db, Now: func() time.Time {
		return time.Date(2026, 8, 12, 12, 34, 56, 123456789, time.UTC)
	}}
	if err := control.Kill(ctx, "epoch-kill-cost"); err != nil {
		t.Fatalf("kill epoch: %v", err)
	}
	var status string
	var reserved, actual int64
	if err := db.QueryRowContext(ctx, `SELECT status, reserved_micro, actual_micro FROM cost_reservations WHERE episode_id = 'epi-kill-cost'`).Scan(&status, &reserved, &actual); err != nil {
		t.Fatalf("read released reservation: %v", err)
	}
	if status != "settled" || reserved != 10 || actual != 0 {
		t.Fatalf("reservation status=%q reserved=%d actual=%d, want settled/10/0", status, reserved, actual)
	}
	if err := db.QueryRowContext(ctx, "SELECT reserved_micro FROM cost_limits WHERE scope_key = 'global'").Scan(&reserved); err != nil {
		t.Fatalf("read global reservation: %v", err)
	}
	if reserved != 0 {
		t.Fatalf("global reserved_micro=%d, want 0", reserved)
	}
}

func TestEpochControlDecisionPathFailsClosedWhenMisconfigured(t *testing.T) {
	ctx := context.Background()
	var nilControl *storage.EpochControl
	if _, err := nilControl.State(ctx, "epoch"); err == nil {
		t.Fatal("nil State receiver returned success")
	}
	if err := nilControl.AssertDecision(ctx, "epoch"); err == nil {
		t.Fatal("nil AssertDecision receiver returned success")
	}

	db, _ := openOwnerDB(t)
	control := &storage.EpochControl{DB: db}
	if _, err := control.State(ctx, ""); err == nil {
		t.Fatal("empty State epoch returned success")
	}
	if err := control.AssertDecision(ctx, ""); err == nil {
		t.Fatal("empty AssertDecision epoch returned success")
	}
	var nilDBControl = &storage.EpochControl{}
	if err := nilDBControl.AssertDecisionTx(ctx, nil, "epoch"); err == nil {
		t.Fatal("nil DB/transactional AssertDecision returned success")
	}
	if err := control.AssertDecisionTx(ctx, nil, "epoch"); err == nil {
		t.Fatal("nil transaction AssertDecision returned success")
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return control.AssertDecisionTx(ctx, tx, "")
	}); err == nil {
		t.Fatal("empty transactional AssertDecision epoch returned success")
	}
}

func seedRunningEpochEpisode(t *testing.T, db *storage.DB, episodeID, epoch string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys for fixture: %v", err)
	}
	digest := make([]byte, 32)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence,
			accepted_at, policy_epoch
		) VALUES (?, ?, 'tenant', ?, 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'running', 1,
			'2026-08-12T10:00:00.000000000Z', ?)`,
		episodeID, "sch-"+episodeID, "sit-"+episodeID, digest, digest, epoch); err != nil {
		t.Fatalf("seed in-flight episode: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version,
			last_reasoned_version, phase, status, first_event_time, latest_event_time,
			updated_at, created_at
		) VALUES (?, 'tenant', 'dep-epoch', 'test', 'thing', 'ent-epoch', 0, ?, 1, 0,
			'candidate', 'open', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`,
		"sit-"+episodeID, "occ-"+episodeID); err != nil {
		t.Fatalf("seed situation registry: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("restore foreign keys: %v", err)
	}
}
