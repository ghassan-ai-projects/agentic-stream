package episodeledger_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestALateOutputOfAnEarlierAttemptIsRefusedAndAuditedAfterARetry(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, t.Context(), db, "epi-fenced")

	var first, second episodeledger.Identity
	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if first, err = episodeledger.StartAttempt(ctx, tx, "epi-fenced", "att-first", now); err != nil {
			return err
		}
		if err := episodeledger.TransitionAttempt(ctx, tx, first, episodeledger.AttemptRunning, now, nil, nil); err != nil {
			return err
		}
		if err := episodeledger.TransitionAttempt(ctx, tx, first, episodeledger.AttemptAbandoned, now.Add(time.Second), []byte(`{"reason":"grace_expired"}`), nil); err != nil {
			return err
		}
		second, err = episodeledger.StartAttempt(ctx, tx, "epi-fenced", "att-second", now.Add(2*time.Second))
		return err
	})
	if second.Fence != first.Fence+1 {
		t.Fatalf("retry fence = %d, want %d", second.Fence, first.Fence+1)
	}

	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := episodeledger.TransitionAttempt(ctx, tx, first, episodeledger.AttemptProduced, now, nil, nil); !reasonIs(err, episodeledger.RejectStaleAttempt) {
			t.Errorf("late output of the first attempt = %v, want stale_attempt", err)
		}
		if err := episodeledger.RecordRejection(ctx, tx, first, episodeledger.RejectStaleAttempt, []byte(`{"same_snapshot":true}`), now.Add(3*time.Second)); err != nil {
			return err
		}
		return episodeledger.TransitionAttempt(ctx, tx, second, episodeledger.AttemptRunning, now.Add(3*time.Second), nil, nil)
	})
	var rejections int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM episode_rejections WHERE episode_id = ? AND reason = ?", "epi-fenced", episodeledger.RejectStaleAttempt).Scan(&rejections); err != nil || rejections != 1 {
		t.Fatalf("stale rejections = %d err=%v, want the one audited refusal", rejections, err)
	}
}

func TestAnOwnerCheckThatReportsALostEpochRefusesTheAttemptAsStale(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, t.Context(), db, "epi-owned")
	lost := func(context.Context, *sql.Tx, string) error {
		return fmt.Errorf("lease taken: %w", episodeledger.ErrOwnerLost)
	}

	inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := episodeledger.StartAttemptOwned(ctx, tx, "epi-owned", "att-1", "epoch-1", lost, now); !reasonIs(err, episodeledger.RejectStaleAttempt) {
			t.Errorf("start under a lost epoch = %v, want stale_attempt", err)
		}
		identity, err := episodeledger.StartAttemptOwned(ctx, tx, "epi-owned", "att-1", "epoch-1", ownerHolds, now)
		if err != nil || identity.OwnerEpoch != "epoch-1" || identity.Fence != 1 {
			t.Errorf("start under a held epoch = %+v err=%v, want fence 1 owned by epoch-1", identity, err)
		}
		if err := episodeledger.TransitionAttempt(ctx, tx, identity, episodeledger.AttemptRunning, now, nil, lost); !reasonIs(err, episodeledger.RejectStaleAttempt) {
			t.Errorf("transition under a lost epoch = %v, want stale_attempt", err)
		}
		return episodeledger.TransitionAttempt(ctx, tx, identity, episodeledger.AttemptRunning, now, nil, ownerHolds)
	})
}

func TestCrashRecoveryAbandonsThePreviousEpochsAttemptAndIsIdempotent(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, t.Context(), db, "epi-recovery")
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, owner_epoch, started_at)
		VALUES ('att-recovery', 'epi-recovery', 1, 'running', 'epoch-old', '2026-08-12T10:00:00Z');
		UPDATE episodes SET lifecycle_status = 'running', current_attempt_id = 'att-recovery', current_fence = 1 WHERE episode_id = 'epi-recovery'`); err != nil {
		t.Fatalf("seed running attempt: %v", err)
	}
	runRecovery := func(at time.Time) (report episodeledger.RecoveryReport) {
		inTx(t, db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			report, err = episodeledger.RecoverUnfinishedAttempts(ctx, tx, "epoch-new", at, nil)
			return err
		})
		return report
	}
	if report := runRecovery(now.Add(time.Hour)); report != (episodeledger.RecoveryReport{AbandonedAttempts: 1, RequeuedEpisodes: 1}) {
		t.Fatalf("recovery report = %+v", report)
	}
	var status, lifecycle, reason string
	if err := db.QueryRowContext(t.Context(), `
		SELECT a.status, e.lifecycle_status, json_extract(a.terminal_json, '$.reason')
		FROM episode_attempts a JOIN episodes e ON e.episode_id = a.episode_id WHERE a.attempt_id = 'att-recovery'`).Scan(&status, &lifecycle, &reason); err != nil {
		t.Fatal(err)
	}
	if status != string(episodeledger.AttemptAbandoned) || lifecycle != string(episodeledger.LifecycleRunning) || reason != "runtime_restart" {
		t.Fatalf("recovered state status=%q lifecycle=%q reason=%q", status, lifecycle, reason)
	}
	if report := runRecovery(now.Add(2 * time.Hour)); report != (episodeledger.RecoveryReport{}) {
		t.Fatalf("repeat recovery report = %+v, want zero", report)
	}
}
