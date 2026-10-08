package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func within(t *testing.T, work func(ctx context.Context, tx *store.Tx)) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		work(t.Context(), store.Join(raw))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func admission(id string) domain.Admission {
	digest := make([]byte, 32)
	return domain.Admission{
		EpisodeID: id, SchedulerItemID: "item-" + id, Kind: "standard", TenantID: "t", SituationID: "s-" + id, SituationVersion: 1,
		ExecutorName: "x", ExecutorVersion: "v", ModelPolicy: "p", PromptVersion: "v1",
		SnapshotSHA256: digest, PromptSHA256: digest, ObjectiveSHA256: digest, AdmissionKey: append([]byte(id), digest[len(id):]...), RequestJSON: []byte("{}"), DispatchPolicy: "shadow",
	}
}

func TestAttemptLifecycleRunsUnderFencing(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := Admit(ctx, tx, admission("e1"), now); err != nil {
			t.Fatal(err)
		}
		identity, err := StartAttempt(ctx, tx, "e1", "a1", now)
		if err != nil || identity.Fence != 1 {
			t.Fatalf("identity=%+v err=%v", identity, err)
		}
		if _, err := StartAttempt(ctx, tx, "e1", "a2", now); err == nil {
			t.Fatal("second active attempt accepted")
		}
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptRunning, now, nil, now); err != nil {
			t.Fatal(err)
		}
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptDispatched, now, nil, now); err == nil {
			t.Fatal("backward transition accepted")
		}
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptProduced, now, []byte("{}"), now); err != nil {
			t.Fatal(err)
		}
		stale := identity
		stale.Fence = 0
		if err := ValidateWorkerIdentity(ctx, tx, stale, now); !domain.IsIdentityReason(err, domain.RejectStaleAttempt) {
			t.Fatalf("stale identity = %v", err)
		}
	})
}

func TestStartingAnAttemptRequiresAnOwnedEpochToHoldTheLease(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := Admit(ctx, tx, admission("e1"), now); err != nil {
			t.Fatal(err)
		}
		if _, err := StartAttemptOwned(ctx, tx, "e1", "a1", "", now); err == nil {
			t.Fatal("owned start without an epoch accepted")
		}
		if _, err := StartAttemptOwned(ctx, tx, "e1", "a1", "epoch", now); !domain.IsIdentityReason(err, domain.RejectStaleAttempt) {
			t.Fatalf("owned start without a lease = %v", err)
		}
		if _, err := StartAttempt(ctx, tx, "", "a1", now); err == nil {
			t.Fatal("start without an episode accepted")
		}
		if _, err := StartAttempt(ctx, tx, "missing", "a1", now); !domain.IsIdentityReason(err, domain.RejectUnknownEpisode) {
			t.Fatalf("unknown episode = %v", err)
		}
	})
}

func TestRecoveryRequiresATransactionAndAnEpoch(t *testing.T) {
	t.Parallel()
	if _, err := RecoverUnfinishedAttempts(t.Context(), store.Join(nil), "epoch", now, nil); err == nil {
		t.Fatal("recovery without a transaction accepted")
	}
	within(t, func(ctx context.Context, tx *store.Tx) {
		if _, err := RecoverUnfinishedAttempts(ctx, tx, "", now, nil); err == nil {
			t.Fatal("recovery without an epoch accepted")
		}
		if err := Admit(ctx, tx, admission("e1"), now); err != nil {
			t.Fatal(err)
		}
		if _, err := StartAttempt(ctx, tx, "e1", "a1", now); err != nil {
			t.Fatal(err)
		}
		report, err := RecoverUnfinishedAttempts(ctx, tx, "new-epoch", now, nil)
		if err != nil || report.AbandonedAttempts != 1 || report.RequeuedEpisodes != 1 {
			t.Fatalf("report=%+v err=%v", report, err)
		}
	})
}

func TestRejectionsRecordUnknownEpisodesWithoutAForeignKey(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := RecordRejection(ctx, tx, domain.Identity{}, "made_up", nil, now); err == nil {
			t.Fatal("unregistered reason accepted")
		}
		identity := domain.Identity{EpisodeID: "missing", AttemptID: "a", Fence: 1}
		for range 2 {
			if err := RecordRejection(ctx, tx, identity, domain.RejectUnknownEpisode, nil, now); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestSchedulerQueueRules(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		item := domain.SchedulerItem{SchedulerItemID: "i", TriggerID: "t1", SituationID: "s", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: now.Add(time.Hour)}
		if err := UpsertSchedulerItem(ctx, tx, item, "tenant", make([]byte, 32), now); err != nil {
			t.Fatal(err)
		}
		other := item
		other.TriggerID = "t2"
		if err := UpsertSchedulerItem(ctx, tx, other, "tenant", append([]byte{1}, make([]byte, 31)...), now); err != nil {
			t.Fatalf("id clash must keep the existing item: %v", err)
		}
		if err := CoalesceSkippedItem(ctx, tx, "i", now); err != nil {
			t.Fatal(err)
		}
		if err := CoalesceSkippedItem(ctx, tx, "i", now); err == nil {
			t.Fatal("coalescing a non-pending item accepted")
		}
		if err := CoalesceCostRejectedItem(ctx, tx, "i", now); err == nil {
			t.Fatal("cost-rejecting a non-pending item accepted")
		}
		if err := MarkSchedulerItemAdmitted(ctx, tx, "i", now); err == nil {
			t.Fatal("admitting a non-pending item accepted")
		}
		if _, found, err := NextPendingSchedulerItem(ctx, tx, "tenant", now); err != nil || found {
			t.Fatalf("found=%v err=%v", found, err)
		}
	})
}

func TestAdmitReportsLiveConflictOnlyForReconsiderations(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		first := admission("e1")
		first.SituationID = "same"
		if err := Admit(ctx, tx, first, now); err != nil {
			t.Fatal(err)
		}
		standard := admission("e2")
		standard.SituationID = "same"
		if err := Admit(ctx, tx, standard, now); err == nil || errors.Is(err, domain.ErrLiveEpisodeConflict) {
			t.Fatalf("standard collision = %v", err)
		}
	})
}

func TestEpisodeLifecycleWritersAndSupersession(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := Admit(ctx, tx, admission("e1"), now); err != nil {
			t.Fatal(err)
		}
		steps := map[string]error{
			"rebind":       Rebind(ctx, tx, "e1", 2, make([]byte, 32), []byte("{}")),
			"bind":         BindRequest(ctx, tx, "e1", []byte("{}")),
			"retain":       RetainForRetry(ctx, tx, "e1"),
			"supersede":    SupersedeEpoch(ctx, tx, "epoch", now),
			"coalesced":    SupersedeCoalesced(ctx, tx, "s-e1", now),
			"conclude":     Conclude(ctx, tx, "e1", now, []byte("{}")),
			"abandon":      Abandon(ctx, tx, "e1", now, []byte("{}")),
			"abandon-bind": AbandonRebind(ctx, tx, "e1", now, []byte("{}")),
		}
		for name, err := range steps {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
		fence, found, err := tx.ReadEpisodeFence(ctx, "e1")
		if err != nil || !found || fence.Lifecycle != domain.LifecycleAbandoned {
			t.Fatalf("fence=%+v found=%v err=%v", fence, found, err)
		}
	})
}

func TestCancellationOfASupersededEpisodeIsAcknowledgedButOutputIsNot(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := Admit(ctx, tx, admission("e1"), now); err != nil {
			t.Fatal(err)
		}
		identity, err := StartAttempt(ctx, tx, "e1", "a1", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptRunning, now, nil, now); err != nil {
			t.Fatal(err)
		}
		if err := SupersedeEpoch(ctx, tx, "", now); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.SetAttemptStatus(ctx, identity, domain.AttemptRunning); err != nil {
			t.Fatal(err)
		}
		if err := SupersedeCoalesced(ctx, tx, "s-e1", now); err != nil {
			t.Fatal(err)
		}
		if err := Abandon(ctx, tx, "e1", now, nil); err != nil {
			t.Fatal(err)
		}
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptProduced, now, nil, now); !domain.IsIdentityReason(err, domain.RejectEpisodeClosed) {
			t.Fatalf("output after closure = %v", err)
		}
		if err := TransitionAttempt(ctx, tx, identity, domain.AttemptCancelled, now, []byte("{}"), now); err != nil {
			t.Fatalf("cancellation acknowledgement = %v", err)
		}
	})
}
