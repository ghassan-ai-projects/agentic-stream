package episodeledger_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestEpisodeMutationsRemainInsideTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, t.Context(), db, "episode")
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("abort composed transition")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		ctx := t.Context()
		if err := episodeledger.Rebind(ctx, tx, "episode", 2, make([]byte, 32), []byte(`{"snapshot":"fresh"}`)); err != nil {
			return err
		}
		if err := episodeledger.BindRequest(ctx, tx, "episode", []byte(`{"attempt":"bound"}`)); err != nil {
			return err
		}
		if err := episodeledger.AbandonRebind(ctx, tx, "episode", now, []byte(`{"reason":"invalid"}`)); err != nil {
			return err
		}
		if err := episodeledger.RetainForRetry(ctx, tx, "episode"); err != nil {
			return err
		}
		if err := episodeledger.Conclude(ctx, tx, "episode", now.Add(time.Second), []byte(`{"status":"declined"}`)); err != nil {
			return err
		}
		if err := episodeledger.Abandon(ctx, tx, "episode", now.Add(2*time.Second), []byte(`{"reason":"killed"}`)); err != nil {
			return err
		}
		var state, ended string
		if err := tx.QueryRowContext(ctx, "SELECT lifecycle_status, ended_at FROM episodes WHERE episode_id='episode'").Scan(&state, &ended); err != nil {
			return err
		}
		if state != "abandoned" || ended != kernel.FormatTime(now.Add(2*time.Second)) {
			t.Fatalf("composed state=%s ended=%s", state, ended)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback: %v", err)
	}
	var state string
	var rebinds, version int
	if err := db.QueryRowContext(t.Context(), "SELECT lifecycle_status, stale_rebind_count, situation_version FROM episodes WHERE episode_id='episode'").Scan(&state, &rebinds, &version); err != nil {
		t.Fatal(err)
	}
	if state != "admitted" || rebinds != 0 || version != 1 {
		t.Fatalf("transition escaped rollback: state=%s rebinds=%d version=%d", state, rebinds, version)
	}
}

func TestAttemptStartTransitionAndRecoveryRollBackWithTheCallersTransaction(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, t.Context(), db, "episode")
	rollback := errors.New("abort composed transition")
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		ctx := t.Context()
		identity, err := episodeledger.StartAttempt(ctx, tx, "episode", "attempt", now)
		if err != nil {
			return err
		}
		if err := episodeledger.TransitionAttempt(ctx, tx, identity, episodeledger.AttemptRunning, now, nil, nil); err != nil {
			return err
		}
		if _, err := episodeledger.RecoverUnfinishedAttempts(ctx, tx, "new-epoch", now, nil); err != nil {
			return err
		}
		if err := episodeledger.RecordRejection(ctx, tx, identity, episodeledger.RejectStaleAttempt, nil, now); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback: %v", err)
	}
	var attempts, rejections int
	if err := db.QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM episode_attempts), (SELECT COUNT(*) FROM episode_rejections)").Scan(&attempts, &rejections); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || rejections != 0 {
		t.Fatalf("attempt writes escaped rollback: attempts=%d rejections=%d", attempts, rejections)
	}
}
