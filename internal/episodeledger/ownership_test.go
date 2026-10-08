package episodeledger_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestAdmissionStoresDeclaredPolicyAndRejectsConflictingLiveEpisode(t *testing.T) {
	db := storagetest.OpenTemp(t)

	seedEpisode(t, t.Context(), db, "original")
	// Isolate the row contract from unrelated upstream situation/scheduler fixtures.
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 2, 12, 0, 0, 123456789, time.UTC)
	admission := episodeledger.Admission{EpisodeID: "next", SchedulerItemID: "sch-next", TenantID: "tenant", SituationID: "other", SituationVersion: 1,
		ExecutorName: "executor", ExecutorVersion: "revision", ModelPolicy: "policy", PromptVersion: "prompt", SnapshotSHA256: make([]byte, 32),
		PromptSHA256: make([]byte, 32), ObjectiveSHA256: make([]byte, 32), AdmissionKey: make([]byte, 32), RequestJSON: []byte(`{}`), PolicyEpoch: "epoch", DispatchPolicy: "shadow"}
	admission.AdmissionKey[0] = 1
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return episodeledger.Admit(t.Context(), tx, admission, now) }); err != nil {
		t.Fatal(err)
	}
	var policy, accepted string
	if err := db.QueryRowContext(t.Context(), "SELECT dispatch_policy, accepted_at FROM episodes WHERE episode_id='next'").Scan(&policy, &accepted); err != nil {
		t.Fatal(err)
	}
	if policy != "shadow" || accepted != "2026-10-02T12:00:00.123456789Z" {
		t.Fatalf("admission policy=%s accepted=%s", policy, accepted)
	}
	admission.EpisodeID = "conflict"
	admission.SchedulerItemID = "sch-conflict"
	admission.Kind = "reconsider"
	admission.SituationID = "sit-test"
	admission.AdmissionKey[0] = 2
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return episodeledger.Admit(t.Context(), tx, admission, now) })
	if !errors.Is(err, episodeledger.ErrLiveEpisodeConflict) {
		t.Fatalf("live episode conflict lost: %v", err)
	}
}

func TestEpisodeMutationsRemainInsideCallerTransaction(t *testing.T) {
	db := storagetest.OpenTemp(t)

	seedEpisode(t, t.Context(), db, "episode")
	// Isolate the row contract from unrelated upstream situation/scheduler fixtures.
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
		if err := episodeledger.AbandonRebind(ctx, tx, "episode", "first", []byte(`{"reason":"invalid"}`)); err != nil {
			return err
		}
		if err := episodeledger.RetainForRetry(ctx, tx, "episode"); err != nil {
			return err
		}
		if err := episodeledger.Conclude(ctx, tx, "episode", "second", []byte(`{"status":"declined"}`)); err != nil {
			return err
		}
		if err := episodeledger.Abandon(ctx, tx, "episode", "final", []byte(`{"reason":"killed"}`)); err != nil {
			return err
		}
		var state, ended string
		var rebinds, version int
		if err := tx.QueryRowContext(ctx, "SELECT lifecycle_status, ended_at, stale_rebind_count, situation_version FROM episodes WHERE episode_id='episode'").Scan(&state, &ended, &rebinds, &version); err != nil {
			return err
		}
		if state != "abandoned" || ended != "final" || rebinds != 2 || version != 2 {
			t.Fatalf("composed state=%s ended=%s rebinds=%d version=%d", state, ended, rebinds, version)
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

func TestCancellationRecoveryAbandonsRatherThanRequeues(t *testing.T) {
	db := storagetest.OpenTemp(t)

	seedEpisode(t, t.Context(), db, "episode")
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		identity, err := episodeledger.StartAttempt(t.Context(), tx, "episode", "attempt", now)
		if err != nil {
			return err
		}
		return episodeledger.TransitionAttempt(t.Context(), tx, identity, episodeledger.AttemptCancelling, now, nil)
	}); err != nil {
		t.Fatal(err)
	}
	var report episodeledger.RecoveryReport
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		report, err = episodeledger.RecoverUnfinishedAttempts(t.Context(), tx, "new-owner", now, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if report.AbandonedAttempts != 1 || report.AbandonedEpisodes != 1 || report.RequeuedEpisodes != 0 {
		t.Fatalf("canceled attempt retried: %+v", report)
	}
	var lifecycle string
	if err := db.QueryRowContext(t.Context(), "SELECT lifecycle_status FROM episodes WHERE episode_id='episode'").Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "abandoned" {
		t.Fatalf("recovered lifecycle=%s", lifecycle)
	}
}

func TestUnknownWorkerRejectionIsDurableAndIdempotent(t *testing.T) {
	db := storagetest.OpenTemp(t)

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	identity := episodeledger.Identity{EpisodeID: "unknown", AttemptID: "forged", Fence: 7}
	for range 2 {
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return episodeledger.RecordRejection(t.Context(), tx, identity, episodeledger.RejectUnknownEpisode, nil, now)
		}); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var episode sql.NullString
	var reason, details string
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*), episode_id, reason, details_json FROM episode_rejections").Scan(&count, &episode, &reason, &details); err != nil {
		t.Fatal(err)
	}
	if count != 1 || episode.Valid || reason != "unknown_episode" || details != "{}" {
		t.Fatalf("rejection count=%d episode=%v reason=%s details=%s", count, episode, reason, details)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return episodeledger.RecordRejection(t.Context(), tx, identity, episodeledger.RejectionReason("invented"), nil, now)
	}); err == nil {
		t.Fatal("unregistered rejection accepted")
	}
}

func TestAdmissionRefusesAnUndeclaredDispatchPolicy(t *testing.T) {
	db := storagetest.OpenTemp(t)

	seedEpisode(t, t.Context(), db, "original")
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	admission := episodeledger.Admission{EpisodeID: "next", SchedulerItemID: "sch-next", Kind: episodeledger.KindStandard, TenantID: "tenant", SituationID: "other", SituationVersion: 1,
		ExecutorName: "executor", ExecutorVersion: "revision", ModelPolicy: "policy", PromptVersion: "prompt", SnapshotSHA256: make([]byte, 32),
		PromptSHA256: make([]byte, 32), ObjectiveSHA256: make([]byte, 32), AdmissionKey: make([]byte, 32), RequestJSON: []byte(`{}`), PolicyEpoch: "epoch"}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return episodeledger.Admit(t.Context(), tx, admission, time.Now()) })
	if err == nil {
		t.Fatal("the ledger admitted an episode with no declared dispatch policy")
	}
}
