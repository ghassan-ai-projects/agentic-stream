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

func TestOpennessAndTimeEncodings(t *testing.T) {
	t.Parallel()
	if store.Join(nil).Open() || store.Reader(nil).Open() {
		t.Fatal("nil handles reported open")
	}
	if got := store.AcceptedAtText(at); got != "2026-08-12T12:00:00.000000000Z" {
		t.Fatalf("accepted_at = %s", got)
	}
	if got := store.TimeText(at); got != "2026-08-12T12:00:00Z" {
		t.Fatalf("time = %s", got)
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
		must(tx.UpsertSchedulerItem(ctx, item("i1", "t1"), "tenant", make([]byte, 32), "now"))
		if err := tx.UpsertSchedulerItem(ctx, item("i1", "t2"), "tenant", append([]byte{1}, make([]byte, 31)...), "now"); !store.IsSchedulerItemIDConflict(err) {
			t.Fatalf("id clash not classified: %v", err)
		}
		must(tx.InsertSchedulerItemIfAbsent(ctx, item("i1", "t2"), "tenant", make([]byte, 32), "now"))
		id, found, err := tx.NextPendingSchedulerItem(ctx, "tenant", at)
		if err != nil || !found || id != "i1" {
			t.Fatalf("next = %q %v %v", id, found, err)
		}
		if rows, err := tx.AdmitPendingSchedulerItem(ctx, "i1", at); err != nil || rows != 1 {
			t.Fatalf("admit rows=%d err=%v", rows, err)
		}
		if rows, _ := tx.AdmitPendingSchedulerItem(ctx, "i1", at); rows != 0 {
			t.Fatalf("second admit rows=%d", rows)
		}
		if _, found, _ := tx.NextPendingSchedulerItem(ctx, "tenant", at); found {
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
		if _, found, err := tx.ReadTerminalEpisodeFence(ctx, "missing"); err != nil || found {
			t.Fatalf("terminal fence found=%v err=%v", found, err)
		}
		if _, found, err := tx.ReadAttempt(ctx, domain.Identity{EpisodeID: "missing", AttemptID: "a"}); err != nil || found {
			t.Fatalf("attempt found=%v err=%v", found, err)
		}
		if known, err := tx.EpisodeExists(ctx, "missing"); err != nil || known {
			t.Fatalf("exists=%v err=%v", known, err)
		}
		if held, err := tx.OwnerHoldsLease(ctx, "e", store.TimeText(at)); err != nil || held {
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
		rejection := domain.Rejection{ID: "rej_1", Reason: domain.RejectUnknownEpisode, Details: []byte("{}"), At: store.TimeText(at)}
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
		ExecutorName: "x", ExecutorVersion: "v", ModelPolicy: "p", PromptVersion: "v1",
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
		must(tx.InsertEpisode(ctx, admitted("e1"), "shadow", store.AcceptedAtText(at)))
		if err := tx.InsertEpisode(ctx, admitted("e1"), "shadow", store.AcceptedAtText(at)); err == nil {
			t.Fatal("duplicate episode accepted")
		}
		identity := domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1}
		must(tx.InsertAttempt(ctx, identity, "now"))
		must(tx.RecordEpisodeAttempt(ctx, identity, "now"))
		fence, found, err := tx.ReadEpisodeFence(ctx, "e1")
		if err != nil || !found || fence.Attempt != "a1" || fence.Fence != 1 || fence.Lifecycle != domain.LifecycleRunning {
			t.Fatalf("fence=%+v found=%v err=%v", fence, found, err)
		}
		if rows, err := tx.StartRunningAttempt(ctx, identity, "later"); err != nil || rows != 1 {
			t.Fatalf("running rows=%d err=%v", rows, err)
		}
		if rows, err := tx.SetAttemptStatus(ctx, identity, domain.AttemptCancelling); err != nil || rows != 1 {
			t.Fatalf("status rows=%d err=%v", rows, err)
		}
		if status, err := tx.ReadAttemptStatusOf(ctx, identity); err != nil || status != domain.AttemptCancelling {
			t.Fatalf("status=%s err=%v", status, err)
		}
		if rows, err := tx.FinishAttempt(ctx, identity, domain.AttemptCancelled, "end", []byte("{}")); err != nil || rows != 1 {
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
		must(tx.ConcludeEpisode(ctx, "e1", "end", []byte("{}")))
		if lifecycle, err := tx.EpisodeLifecycle(ctx, "e1"); err != nil || lifecycle != domain.LifecycleConcluded {
			t.Fatalf("lifecycle=%s err=%v", lifecycle, err)
		}
	})
}

func TestSupersessionAndRecoveryWritesReportTheirRows(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx) {
		if err := tx.InsertEpisode(ctx, admitted("e1"), "shadow", store.AcceptedAtText(at)); err != nil {
			t.Fatal(err)
		}
		identity := domain.Identity{EpisodeID: "e1", AttemptID: "a1", Fence: 1}
		if err := tx.InsertAttempt(ctx, identity, "now"); err != nil {
			t.Fatal(err)
		}
		attempts, err := tx.UnfinishedAttempts(ctx, "new-epoch")
		if err != nil || len(attempts) != 1 || attempts[0].AttemptID != "a1" || attempts[0].Status != domain.AttemptDispatched {
			t.Fatalf("attempts=%+v err=%v", attempts, err)
		}
		if rows, err := tx.AbandonUnfinishedAttempt(ctx, "a1", "e1", "end", []byte("{}")); err != nil || rows != 1 {
			t.Fatalf("abandon attempt rows=%d err=%v", rows, err)
		}
		if rows, _ := tx.AbandonUnfinishedAttempt(ctx, "a1", "e1", "end", []byte("{}")); rows != 0 {
			t.Fatalf("second abandon rows=%d", rows)
		}
		if rows, err := tx.AbandonOpenEpisode(ctx, "e1", "end", []byte("{}")); err != nil || rows != 1 {
			t.Fatalf("abandon episode rows=%d err=%v", rows, err)
		}
		settler := &recordingSettler{}
		if err := tx.SettleEpisodeCost(ctx, settler, "e1", "now"); err != nil || len(settler.episodes) != 1 {
			t.Fatalf("settled=%v err=%v", settler.episodes, err)
		}
		for name, err := range map[string]error{
			"epoch":     tx.SupersedeEpochEpisodes(ctx, "epoch", "now"),
			"coalesced": tx.SupersedeCoalescedEpisodes(ctx, "s-e1", "now"),
			"attempts":  tx.CancelCoalescedAttempts(ctx, "s-e1"),
			"trigger":   tx.CoalesceTriggerItems(ctx, "s", "alarm", "now"),
			"abandon":   tx.AbandonEpisode(ctx, "e1", "now", nil),
			"rebind":    tx.AbandonRebind(ctx, "e1", "now", nil),
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
		if err := tx.UpsertSchedulerItem(ctx, item("i1", "t1"), "tenant", make([]byte, 32), "now"); err != nil {
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
		if err := tx.InsertEpisode(ctx, first, "shadow", "t"); err != nil {
			t.Fatal(err)
		}
		if err := tx.InsertEpisode(ctx, second, "shadow", "t"); !store.IsLiveEpisodeViolation(err) {
			t.Fatalf("live violation not classified: %v", err)
		}
	})
}
