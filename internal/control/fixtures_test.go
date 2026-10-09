package control_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"strings"
	"testing"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var epoch0 = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func openOwnerDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)
	db.SetMaxOpenConns(1)
	return db
}

func ownerOn(db *storage.DB, instance string, clock *sources.Virtual) *runtimecontrol.RuntimeOwner {
	return &runtimecontrol.RuntimeOwner{DB: db, InstanceID: instance, Lease: time.Minute, Now: clock.Now}
}

func assertRefusal(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}

func assertCostTotals(t *testing.T, db *storage.DB, reserved, spent, killed int) {
	t.Helper()
	var gotReserved, gotSpent, gotKilled int
	if err := db.QueryRowContext(t.Context(), "SELECT reserved_micro, spent_micro, kill_switch FROM cost_limits WHERE scope_key = 'global'").Scan(&gotReserved, &gotSpent, &gotKilled); err != nil {
		t.Fatalf("read global accounting: %v", err)
	}
	if gotReserved != reserved || gotSpent != spent || gotKilled != killed {
		t.Fatalf("global accounting (reserved, spent, kill switch) = (%d,%d,%d), want (%d,%d,%d)", gotReserved, gotSpent, gotKilled, reserved, spent, killed)
	}
}

func fenced(t *testing.T, db *storage.DB, check func(context.Context, *sql.Tx, string) error, epoch string) error {
	t.Helper()
	return db.WithTx(t.Context(), func(tx *sql.Tx) error { return check(t.Context(), tx, epoch) })
}

func seedEpisode(t *testing.T, db *storage.DB, episodeID, epoch, lifecycle string) {
	t.Helper()
	sum := sha256.Sum256([]byte(episodeID))
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence,
			accepted_at, policy_epoch
		) VALUES (?, ?, 'tenant', ?, 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', ?, 1,
			'2026-08-12T10:00:00.000000000Z', ?)`,
		episodeID, "sch-"+episodeID, "sit-"+episodeID, sum[:], sum[:], lifecycle, epoch); err != nil {
		t.Fatalf("seed episode %s: %v", episodeID, err)
	}
}
