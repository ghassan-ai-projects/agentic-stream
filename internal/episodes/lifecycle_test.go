package episodes

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestCanTransitionAttempt(t *testing.T) {
	tests := []struct {
		from AttemptStatus
		to   AttemptStatus
		want bool
	}{
		{AttemptDispatched, AttemptRunning, true},
		{AttemptDispatched, AttemptFailed, true},
		{AttemptRunning, AttemptProduced, true},
		{AttemptRunning, AttemptDeclined, true},
		{AttemptRunning, AttemptCancelling, true},
		{AttemptCancelling, AttemptCancelled, true},
		{AttemptCancelling, AttemptAbandoned, true},
		{AttemptProduced, AttemptFailed, false},
		{AttemptDeclined, AttemptRunning, false},
		{AttemptCancelled, AttemptProduced, false},
	}
	for _, test := range tests {
		t.Run(string(test.from)+"_to_"+string(test.to), func(t *testing.T) {
			if got := CanTransitionAttempt(test.from, test.to); got != test.want {
				t.Fatalf("CanTransitionAttempt(%q, %q) = %t, want %t", test.from, test.to, got, test.want)
			}
		})
	}
}

func TestFencingRejectsLateOutputWithIdenticalSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	seedEpisode(t, ctx, db, "epi-fenced")

	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	var first Identity
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		first, err = StartAttempt(ctx, tx, "epi-fenced", "att-first", now)
		if err != nil {
			return err
		}
		if err := TransitionAttempt(ctx, tx, first, AttemptRunning, now, nil); err != nil {
			return err
		}
		return TransitionAttempt(ctx, tx, first, AttemptAbandoned, now.Add(time.Second), []byte(`{"reason":"grace_expired"}`))
	}); err != nil {
		t.Fatalf("finish first attempt: %v", err)
	}

	var second Identity
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		second, err = StartAttempt(ctx, tx, "epi-fenced", "att-second", now.Add(2*time.Second))
		return err
	}); err != nil {
		t.Fatalf("start retry: %v", err)
	}
	if second.Fence != first.Fence+1 {
		t.Fatalf("retry fence = %d, want %d", second.Fence, first.Fence+1)
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := ValidateWorkerIdentity(ctx, tx, first); !IsIdentityReason(err, RejectStaleAttempt) {
			return testErrorf("late first attempt error = %v, want stale_attempt", err)
		}
		if err := RecordRejection(ctx, tx, first, RejectStaleAttempt, []byte(`{"same_snapshot":true}`), now.Add(3*time.Second)); err != nil {
			return err
		}
		return ValidateWorkerIdentity(ctx, tx, second)
	}); err != nil {
		t.Fatalf("validate fenced identities: %v", err)
	}

	var rejections int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM episode_rejections WHERE episode_id = ? AND reason = ?", "epi-fenced", RejectStaleAttempt).Scan(&rejections); err != nil {
		t.Fatalf("count rejection: %v", err)
	}
	if rejections != 1 {
		t.Fatalf("rejection count = %d, want 1", rejections)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return RecordRejection(ctx, tx, Identity{EpisodeID: "missing-episode", AttemptID: "att-missing"}, RejectUnknownEpisode, nil, now.Add(4*time.Second))
	}); err != nil {
		t.Fatalf("record unknown-episode rejection: %v", err)
	}
	var unknownRejections int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM episode_rejections WHERE episode_id IS NULL AND reason = ?", RejectUnknownEpisode).Scan(&unknownRejections); err != nil {
		t.Fatalf("count unknown-episode rejection: %v", err)
	}
	if unknownRejections != 1 {
		t.Fatalf("unknown-episode rejection count = %d, want 1", unknownRejections)
	}

	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'closed' WHERE episode_id = ?", "epi-fenced"); err != nil {
			return err
		}
		if err := ValidateWorkerIdentity(ctx, tx, second); !IsIdentityReason(err, RejectEpisodeClosed) {
			return testErrorf("closed episode error = %v, want episode_closed", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("validate closed episode: %v", err)
	}
}

func seedEpisode(t *testing.T, ctx context.Context, db *storage.DB, episodeID string) {
	t.Helper()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	digest := make([]byte, 32)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES (?, 'sch-test', 'tenant', 'sit-test', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'admitted', 0, ?)`,
		episodeID, digest, digest, "2026-08-12T10:00:00Z"); err != nil {
		t.Fatalf("seed episode: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

func testErrorf(format string, args ...any) error {
	return testError(fmt.Sprintf(format, args...))
}
