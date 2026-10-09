package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestJoinedTransactionKeepsCallerOwnership(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	rollback := errors.New("rollback entire admission")
	var received *sql.Tx
	err := db.WithTx(t.Context(), func(original *sql.Tx) error {
		check := func(ctx context.Context, tx *sql.Tx, epoch string) error {
			received = tx
			_, insertErr := tx.ExecContext(ctx, `INSERT INTO epoch_control(epoch,state,updated_at) VALUES (?,'draining','2026-10-05T00:00:00Z')`, epoch)
			return insertErr
		}
		if err := store.Join(original).AssertDecisionEpoch(t.Context(), check, "epoch"); err != nil {
			return err
		}
		if received != original {
			t.Fatal("the epoch check ran on another transaction than the caller's")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM epoch_control WHERE epoch = 'epoch'"); got != 0 {
		t.Fatalf("joined work escaped the caller's rollback: %d rows", got)
	}
}

func TestLifecycleHandoffRollsBackWithTheDecisionUnit(t *testing.T) {
	t.Parallel()
	db := replayedStore(t)
	episodeID := admittedEpisodeID(t, db)
	rejected := errors.New("decision persistence rejected")
	err := store.New(db).WithTx(t.Context(), func(tx *store.Tx) error {
		identity, err := tx.StartAttempt(t.Context(), episodeID, "rollback-attempt", time.Unix(0, 0))
		if err != nil {
			return err
		}
		steps := []func() error{
			func() error {
				return tx.TransitionAttempt(t.Context(), identity, episodeledger.AttemptRunning, time.Unix(1, 0), nil)
			},
			func() error {
				return tx.BindRequest(t.Context(), episodeID, []byte(`{"attempt_id":"rollback-attempt"}`))
			},
			func() error {
				return tx.RecordRejection(t.Context(), identity, episodeledger.RejectWrongAttempt, []byte(`{}`), time.Unix(2, 0))
			},
			func() error {
				return tx.TransitionAttempt(t.Context(), identity, episodeledger.AttemptFailed, time.Unix(3, 0), []byte(`{}`))
			},
			func() error { return tx.RetainForRetry(t.Context(), episodeID) },
			func() error { return tx.Conclude(t.Context(), episodeID, lifecycleTime, []byte(`{}`)) },
		}
		for _, step := range steps {
			if err := step(); err != nil {
				return err
			}
		}
		return rejected
	})
	if !errors.Is(err, rejected) {
		t.Fatalf("rollback = %v", err)
	}
	if got := scalar[string](t, db, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID); got != "admitted" {
		t.Fatalf("lifecycle escaped the rollback: %s", got)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM episode_attempts WHERE episode_id = ?", episodeID); got != 0 {
		t.Fatalf("attempts escaped the rollback: %d", got)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM episode_rejections WHERE episode_id = ?", episodeID); got != 0 {
		t.Fatalf("rejections escaped the rollback: %d", got)
	}
}

func TestShadowDecisionSharesTheCallerTransactionAndNeverCreatesActionRecords(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	decision := domain.ShadowDecision{
		ShadowDecisionID: "shadow", EpisodeID: "episode", DecisionID: "decision", AttemptID: "attempt",
		DecisionJSON: []byte(`{}`), DecisionSHA256: make([]byte, 32), ShadowScore: domain.ShadowWouldApprove, ScoreReason: "would_approve_R1",
		TenantID: "tenant", SituationID: "situation", SituationVersion: 1, PolicyEpoch: "epoch",
	}
	rollback := errors.New("caller failed after recording evidence")
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		if err := store.Join(raw).RecordShadowDecision(t.Context(), decision, lifecycleTime); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("record then rollback: %v", err)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM shadow_decisions"); got != 0 {
		t.Fatalf("shadow decisions after the caller rolled back = %d, want 0", got)
	}
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		return store.Join(raw).RecordShadowDecision(t.Context(), decision, lifecycleTime)
	}); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"shadow_decisions": 1, "intents": 0, "commands": 0, "outbox": 0} {
		if got := scalar[int](t, db, "SELECT COUNT(*) FROM "+table); got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}
	var score, reason string
	if err := db.QueryRowContext(t.Context(), "SELECT shadow_score, score_reason FROM shadow_decisions").Scan(&score, &reason); err != nil {
		t.Fatal(err)
	}
	if score != "would_approve" || reason != "would_approve_R1" {
		t.Fatalf("shadow decision = (%q, %q), want would_approve for would_approve_R1", score, reason)
	}
}

func TestStoreReportsWhetherItIsConfiguredAndFenced(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	owner := func(context.Context, *sql.Tx, string) error { return nil }
	tests := []struct {
		name       string
		store      store.Store
		configured bool
		fenced     bool
	}{
		{"without a database", store.Store{}, false, false},
		{"with a database", store.New(db), true, false},
		{"fenced by a runtime owner check", store.New(db).Fenced(owner), true, true},
		{"fencing nothing stays unfenced", store.New(db).Fenced(nil), true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.store.Configured() != tc.configured || tc.store.IsFenced() != tc.fenced {
				t.Fatalf("Configured=%v IsFenced=%v, want %v %v", tc.store.Configured(), tc.store.IsFenced(), tc.configured, tc.fenced)
			}
		})
	}
}
