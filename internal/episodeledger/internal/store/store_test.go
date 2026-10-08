package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var at = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func within(t *testing.T, work func(ctx context.Context, tx *store.Tx)) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	// Situation and trigger provenance is covered by cognition tests; isolate the ledger here.

	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		work(t.Context(), store.Join(raw))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func item(id, trigger string) domain.SchedulerItem {
	return domain.SchedulerItem{SchedulerItemID: id, TriggerID: trigger, SituationID: "s", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: at.Add(time.Hour)}
}

func TestOpenness(t *testing.T) {
	t.Parallel()
	if store.Join(nil).Open() || store.Reader(nil).Open() {
		t.Fatal("nil handles reported open")
	}
}

func TestSchedulerQueueRoundTrip(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		must(tx.UpsertSchedulerItem(ctx, item("i1", "t1"), "tenant", make([]byte, 32), at))
		if err := tx.UpsertSchedulerItem(ctx, item("i1", "t2"), "tenant", append([]byte{1}, make([]byte, 31)...), at); !store.IsSchedulerItemIDConflict(err) {
			t.Fatalf("id clash not classified: %v", err)
		}
		must(tx.InsertSchedulerItemIfAbsent(ctx, item("i1", "t2"), "tenant", make([]byte, 32), at))
		pending, err := tx.PendingQueuedItems(ctx, "tenant")
		if err != nil || len(pending) != 1 || pending[0].SchedulerItemID != "i1" || !pending[0].CreatedAt.Equal(at) || pending[0].NotBefore != nil {
			t.Fatalf("pending = %+v %v", pending, err)
		}
		if rows, err := tx.AdmitPendingSchedulerItem(ctx, "i1", at); err != nil || rows != 1 {
			t.Fatalf("admit rows=%d err=%v", rows, err)
		}
		if rows, _ := tx.AdmitPendingSchedulerItem(ctx, "i1", at); rows != 0 {
			t.Fatalf("second admit rows=%d", rows)
		}
		if pending, _ := tx.PendingQueuedItems(ctx, "tenant"); len(pending) != 0 {
			t.Fatal("admitted item still pending")
		}
	})
}

func TestEpisodeFenceReadsReportUnknownEpisodes(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if _, found, err := tx.ReadEpisodeFence(ctx, "missing"); err != nil || found {
			t.Fatalf("fence found=%v err=%v", found, err)
		}
		if _, found, err := tx.ReadAttempt(ctx, domain.Identity{EpisodeID: "missing", AttemptID: "a"}); err != nil || found {
			t.Fatalf("attempt found=%v err=%v", found, err)
		}
		if known, err := tx.EpisodeExists(ctx, "missing"); err != nil || known {
			t.Fatalf("exists=%v err=%v", known, err)
		}
		if held, err := tx.OwnerHoldsLease(ctx, "e", at); err != nil || held {
			t.Fatalf("held=%v err=%v", held, err)
		}
	})
}

func TestMissingAttemptsAreErrorsAndRejectionsAreIdempotent(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if _, err := tx.ReadAttemptStatus(ctx, "missing"); err == nil {
			t.Fatal("missing attempt status accepted")
		}
		if _, err := tx.ReadEpisodeAttemptStatus(ctx, "e", "missing"); err == nil {
			t.Fatal("missing episode attempt status accepted")
		}
		rejection := domain.Rejection{ID: "rej_1", Reason: domain.RejectUnknownEpisode, Details: []byte("{}"), At: at}
		for range 2 {
			if err := tx.InsertRejection(ctx, rejection, false); err != nil {
				t.Fatal(err)
			}
		}
		if attempts, err := tx.UnfinishedAttempts(ctx, "e"); err != nil || len(attempts) != 0 {
			t.Fatalf("attempts=%v err=%v", attempts, err)
		}
	})
}

type recordingSettler struct{ episodes []string }

func (s *recordingSettler) Settle(_ context.Context, tx *sql.Tx, episodeID string, _ uint64, _ string) error {
	if tx == nil {
		return sql.ErrTxDone
	}
	s.episodes = append(s.episodes, episodeID)
	return nil
}

func admitted(id string) domain.Admission {
	digest := make([]byte, 32)
	return domain.Admission{
		EpisodeID: id, SchedulerItemID: "item-" + id, Kind: "standard", TenantID: "t", SituationID: "s-" + id, SituationVersion: 1,
		ExecutorName: "x", ExecutorVersion: "v", ModelPolicy: "p", PromptVersion: "v1", DispatchPolicy: "shadow",
		SnapshotSHA256: digest, PromptSHA256: digest, ObjectiveSHA256: digest, AdmissionKey: append([]byte(id), digest[len(id):]...), RequestJSON: []byte("{}"),
	}
}

func TestEpisodeAndAttemptWritesAreReadBack(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		must(tx.InsertEpisode(ctx, admitted("e1"), at))
		if err := tx.InsertEpisode(ctx, admitted("e1"), at); err == nil {
			t.Fatal("duplicate episode accepted")
		}
		identity := domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1}
		must(tx.InsertAttempt(ctx, identity, at))
		must(tx.RecordEpisodeAttempt(ctx, identity, at))
		fence, found, err := tx.ReadEpisodeFence(ctx, "e1")
		if err != nil || !found || fence.Attempt != "a1" || fence.Fence != 1 || fence.Lifecycle != domain.LifecycleRunning {
			t.Fatalf("fence=%+v found=%v err=%v", fence, found, err)
		}
		if rows, err := tx.StartRunningAttempt(ctx, identity, at.Add(time.Hour)); err != nil || rows != 1 {
			t.Fatalf("running rows=%d err=%v", rows, err)
		}
		if rows, err := tx.SetAttemptStatus(ctx, identity, domain.AttemptCancelling); err != nil || rows != 1 {
			t.Fatalf("status rows=%d err=%v", rows, err)
		}
		if record, _, err := tx.ReadAttempt(ctx, identity); err != nil || record.Status != domain.AttemptCancelling {
			t.Fatalf("status=%s err=%v", record.Status, err)
		}
		if rows, err := tx.FinishAttempt(ctx, identity, domain.AttemptCancelled, at.Add(2*time.Hour), []byte("{}")); err != nil || rows != 1 {
			t.Fatalf("finish rows=%d err=%v", rows, err)
		}
		record, found, err := tx.ReadAttempt(ctx, identity)
		if err != nil || !found || record.Status != domain.AttemptCancelled || record.HasOwnerEpoch {
			t.Fatalf("record=%+v found=%v err=%v", record, found, err)
		}
		if status, err := tx.ReadEpisodeAttemptStatus(ctx, "e1", "a1"); err != nil || status != domain.AttemptCancelled {
			t.Fatalf("status=%s err=%v", status, err)
		}
		must(tx.RebindEpisode(ctx, "e1", 2, make([]byte, 32), []byte("{}")))
		must(tx.BindRequest(ctx, "e1", []byte("{}")))
		must(tx.RetainEpisodeForRetry(ctx, "e1"))
		must(tx.ConcludeEpisode(ctx, "e1", at.Add(2*time.Hour), []byte("{}")))
		if lifecycle, err := tx.EpisodeLifecycle(ctx, "e1"); err != nil || lifecycle != domain.LifecycleConcluded {
			t.Fatalf("lifecycle=%s err=%v", lifecycle, err)
		}
	})
}

func TestSupersessionAndRecoveryWritesReportTheirRows(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := tx.InsertEpisode(ctx, admitted("e1"), at); err != nil {
			t.Fatal(err)
		}
		identity := domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1}
		if err := tx.InsertAttempt(ctx, identity, at); err != nil {
			t.Fatal(err)
		}
		attempts, err := tx.UnfinishedAttempts(ctx, "new-epoch")
		if err != nil || len(attempts) != 1 || attempts[0].AttemptID != "a1" || attempts[0].Status != domain.AttemptDispatched {
			t.Fatalf("attempts=%+v err=%v", attempts, err)
		}
		if rows, err := tx.AbandonUnfinishedAttempt(ctx, "a1", "e1", at.Add(2*time.Hour), []byte("{}")); err != nil || rows != 1 {
			t.Fatalf("abandon attempt rows=%d err=%v", rows, err)
		}
		if rows, _ := tx.AbandonUnfinishedAttempt(ctx, "a1", "e1", at.Add(2*time.Hour), []byte("{}")); rows != 0 {
			t.Fatalf("second abandon rows=%d", rows)
		}
		if rows, err := tx.AbandonOpenEpisode(ctx, "e1", at.Add(2*time.Hour), []byte("{}")); err != nil || rows != 1 {
			t.Fatalf("abandon episode rows=%d err=%v", rows, err)
		}
		settler := &recordingSettler{}
		if err := tx.SettleEpisodeCost(ctx, settler, "e1", at); err != nil || len(settler.episodes) != 1 {
			t.Fatalf("settled=%v err=%v", settler.episodes, err)
		}
		if _, err := tx.CoalesceTriggerItems(ctx, "s", "alarm", at); err != nil {
			t.Errorf("trigger: %v", err)
		}
		for name, err := range map[string]error{
			"epoch":     tx.SupersedeEpochEpisodes(ctx, "epoch", at),
			"coalesced": tx.SupersedeCoalescedEpisodes(ctx, "s-e1", at),
			"attempts":  tx.CancelCoalescedAttempts(ctx, "s-e1"),
			"abandon":   tx.AbandonEpisode(ctx, "e1", at, nil),
			"rebind":    tx.AbandonRebind(ctx, "e1", at, nil),
		} {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}

func TestCoalescePendingReportsRowsAndLiveViolationsAreClassified(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := tx.UpsertSchedulerItem(ctx, item("i1", "t1"), "tenant", make([]byte, 32), at); err != nil {
			t.Fatal(err)
		}
		if rows, err := tx.CoalescePendingSchedulerItem(ctx, "i1", at, "coalesce scheduler item"); err != nil || rows != 1 {
			t.Fatalf("rows=%d err=%v", rows, err)
		}
		if rows, _ := tx.CoalescePendingSchedulerItem(ctx, "i1", at, "coalesce scheduler item"); rows != 0 {
			t.Fatalf("second coalesce rows=%d", rows)
		}
		first, second := admitted("e1"), admitted("e2")
		first.SituationID, second.SituationID = "same", "same"
		if err := tx.InsertEpisode(ctx, first, at); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertEpisode(ctx, second, at); !store.IsLiveEpisodeViolation(err) {
			t.Fatalf("live violation not classified: %v", err)
		}
	})
}
