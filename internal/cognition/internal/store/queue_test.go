package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func TestOnlyPendingItemsOfTheTenantCountTowardCapacity(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	for i, status := range []string{"pending", "pending", "admitted", "coalesced", "expired", "completed"} {
		id := string(rune('a' + i))
		seedEvaluation(t, db, evaluationSeed{triggerID: "trg-" + id, name: "hot", situationID: "sit-" + id, outcome: "admitted", version: 1})
		seedItem(t, db, "sch-"+id, "trg-"+id, "sit-"+id, 1, status)
	}
	exec(t, db, "UPDATE scheduler_items SET tenant_id = 'other' WHERE scheduler_item_id = 'sch-b'")
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		got, err := tx.CountPending(ctx, tenant)
		if err != nil || got != 1 {
			t.Fatalf("CountPending = %d, %v; want 1 (one pending item of this tenant)", got, err)
		}
		return nil
	})
}

func TestPendingItemsOfTheSameSituationAndTriggerAreTheOnesAReplacementMayDisplace(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	for _, item := range []struct{ id, trigger, situation, status string }{
		{"a", "hot", "sit-1", "pending"},
		{"b", "hot", "sit-1", "admitted"},
		{"c", "cold", "sit-1", "pending"},
		{"d", "hot", "sit-2", "pending"},
	} {
		seedEvaluation(t, db, evaluationSeed{triggerID: "trg-" + item.id, name: item.trigger, situationID: item.situation, outcome: "admitted", version: 1})
		seedItem(t, db, "sch-"+item.id, "trg-"+item.id, item.situation, 1, item.status)
	}
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		got, err := tx.CountPendingSameTrigger(ctx, "sit-1", "hot")
		if err != nil || got != 1 {
			t.Fatalf("CountPendingSameTrigger = %d, %v; want 1", got, err)
		}
		none, err := tx.CountPendingSameTrigger(ctx, "sit-1", "unknown")
		if err != nil || none != 0 {
			t.Fatalf("CountPendingSameTrigger(unknown) = %d, %v; want 0", none, err)
		}
		return nil
	})
}

func TestInsertItemRefreshesTheItemAlreadyQueuedForTheSameTrigger(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedEvaluation(t, db, evaluationSeed{triggerID: "trg-1", name: "hot", situationID: "sit-1", outcome: "admitted", version: 1})
	seedItem(t, db, "sch-1", "trg-1", "sit-1", 1, "pending")
	seedItem(t, db, "sch-1", "trg-1", "sit-1", 1, "pending")
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM scheduler_items"); got != 1 {
		t.Fatalf("scheduler items = %d, want 1: re-evaluation must not queue the work twice", got)
	}
}

func TestLatestAdmissionIsTheNewestAdmittedEvaluationOfTheSameTriggerExceptTheCurrentOne(t *testing.T) {
	t.Parallel()
	at := func(minutes int) string { return kernel.FormatTime(now.Add(time.Duration(minutes) * time.Minute)) }
	db := openDB(t)
	for _, e := range []evaluationSeed{
		{triggerID: "old", name: "hot", situationID: "sit-1", outcome: "admitted", version: 1, evaluatedAt: at(-30)},
		{triggerID: "recent", name: "hot", situationID: "sit-1", outcome: "admitted", version: 2, evaluatedAt: at(-10)},
		{triggerID: "current", name: "hot", situationID: "sit-1", outcome: "admitted", version: 3, evaluatedAt: at(0)},
		{triggerID: "ignored", name: "hot", situationID: "sit-1", outcome: "ignored", version: 4, evaluatedAt: at(-1)},
		{triggerID: "other-trigger", name: "cold", situationID: "sit-1", outcome: "admitted", version: 5, evaluatedAt: at(-2)},
		{triggerID: "other-situation", name: "hot", situationID: "sit-2", outcome: "admitted", version: 1, evaluatedAt: at(-3)},
	} {
		seedEvaluation(t, db, e)
	}
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		got, err := tx.LatestAdmittedTime(ctx, "sit-1", "hot", "current")
		if err != nil || got == nil || !got.Equal(now.Add(-10*time.Minute)) {
			t.Fatalf("LatestAdmittedTime = %v, %v; want %s", got, err, now.Add(-10*time.Minute))
		}
		none, err := tx.LatestAdmittedTime(ctx, "sit-9", "hot", "current")
		if err != nil || none != nil {
			t.Fatalf("LatestAdmittedTime for a never-admitted situation = %v, %v; want nil", none, err)
		}
		return nil
	})
}

func TestLatestAdmissionRefusesAnUnreadableEvaluationTime(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedEvaluation(t, db, evaluationSeed{triggerID: "bad", name: "hot", situationID: "sit-1", outcome: "admitted", version: 1, evaluatedAt: "not a time"})
	err := inTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.LatestAdmittedTime(ctx, "sit-1", "hot", "current")
		return err
	})
	requireErrorContaining(t, err, "parse evaluated_at")
}
