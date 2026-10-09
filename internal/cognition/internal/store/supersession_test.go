package store_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func TestReplacementIsTheSituationsCurrentVersionWithItsTrace(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	seedSituation(t, db, "sit-1", 3, 3)
	seedVersion(t, db, versionSeed{situationID: "sit-1", version: 3, traceparent: validTraceparent})
	seedSituation(t, db, "sit-2", 4, 4)
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		got, err := tx.LoadReplacement(ctx, "sit-1")
		want := store.ReplacementVersion{SituationID: "sit-1", TenantID: tenant, Version: 3, Traceparent: validTraceparent}
		if err != nil || got != want {
			t.Fatalf("LoadReplacement = %+v, %v; want %+v", got, err, want)
		}
		noTrace, err := tx.LoadReplacement(ctx, "sit-2")
		if err != nil || noTrace.Version != 4 || noTrace.Traceparent != "" {
			t.Fatalf("a current version without a stored row: %+v, %v; want version 4 and no trace", noTrace, err)
		}
		return nil
	})
}

func TestReplacementOfAnUnknownSituationIsRefused(t *testing.T) {
	t.Parallel()
	err := inTx(t, openDB(t), func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.LoadReplacement(ctx, "sit-unknown")
		return err
	})
	requireErrorContaining(t, err, "load supersession situation")
}

func TestOnlyThisTriggersLiveItemsOfTheSituationAreCoalesced(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	for _, item := range []struct {
		id, trigger, situation, status string
		version                        int
	}{
		{"a", "hot", "sit-1", "pending", 1},
		{"b", "hot", "sit-1", "pending", 2},
		{"c", "cold", "sit-1", "pending", 1},
		{"d", "hot", "sit-2", "pending", 1},
		{"e", "hot", "sit-1", "completed", 1},
	} {
		seedEvaluation(t, db, evaluationSeed{triggerID: "trg-" + item.id, name: item.trigger, situationID: item.situation, outcome: "admitted", version: item.version})
		seedItem(t, db, "sch-"+item.id, "trg-"+item.id, item.situation, item.version, item.status)
	}
	var coalesced []store.SupersededItem
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		var err error
		coalesced, err = tx.CoalesceTriggerWork(ctx, "sit-1", "hot", now)
		return err
	})
	ids := []string{}
	for _, item := range coalesced {
		ids = append(ids, item.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"sch-a", "sch-b"}) {
		t.Fatalf("coalesced = %v, want sch-a and sch-b", ids)
	}
	for id, want := range map[string]string{"sch-a": "coalesced", "sch-b": "coalesced", "sch-c": "pending", "sch-d": "pending", "sch-e": "completed"} {
		if got := scalar[string](t, db, "SELECT status FROM scheduler_items WHERE scheduler_item_id = ?", id); got != want {
			t.Errorf("%s status = %s, want %s", id, got, want)
		}
	}
}

func TestSupersededItemIsAnnouncedWithItsReplacementOnce(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	replacement := store.ReplacementVersion{SituationID: "sit-1", TenantID: tenant, Version: 3}
	item := store.SupersededItem{ID: "sch-1", Version: 2}
	for range 2 {
		mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
			return tx.AnnounceSupersededItem(ctx, replacement, item, now)
		})
	}
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM notifications WHERE event_id LIKE 'situation.superseded:%'"); rows != 1 {
		t.Fatalf("situation.superseded notifications = %d, want 1", rows)
	}
	event := scalar[string](t, db, "SELECT CAST(event_json AS TEXT) FROM notifications WHERE event_id LIKE 'situation.superseded:%'")
	for _, want := range []string{`"superseded_version":2`, `"replacement_version":3`, `"newer_situation_version_admitted"`} {
		if !contains(event, want) {
			t.Errorf("event lacks %s: %s", want, event)
		}
	}
}

func TestSupersedingWithNoPendingApprovalsWithdrawsNothing(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.WithdrawSuperseded(ctx, "sit-1", tenant, 3, now, sources.NewVirtual(now))
	})
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM notifications"); rows != 0 {
		t.Fatalf("notifications = %d, want none", rows)
	}
}

func TestSupersedingWithdrawsOlderPendingApprovalsAndAnnouncesEach(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	for _, intent := range []struct {
		id      string
		version int
		status  string
	}{{"int-old", 1, "pending"}, {"int-current", 3, "pending"}, {"int-decided", 1, "approved"}} {
		exec(t, db, `INSERT INTO decisions (decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version, raw_json, decision_sha256, validation_status, validation_json, created_at)
			VALUES ('dec-'||?, 'epi-'||?, 'att', 1, 1, 'sit-1', ?, X'7B7D', ?, 'accepted', X'7B7D', ?)`, intent.id, intent.id, intent.version, make([]byte, 32), kernel.FormatTime(now))
		exec(t, db, `INSERT INTO intents (intent_id, decision_id, tenant_id, situation_id, situation_version, intent_type, risk_class, intent_json, intent_sha256, expires_at, policy_status, created_at, updated_at)
			VALUES (?, 'dec-'||?, ?, 'sit-1', ?, 'ticket', 'R1', X'7B7D', ?, ?, 'approved', ?, ?)`, intent.id, intent.id, tenant, intent.version, make([]byte, 32), kernel.FormatTime(now.Add(time.Hour)), kernel.FormatTime(now), kernel.FormatTime(now))
		exec(t, db, `INSERT INTO approvals (approval_id, intent_id, status, requested_at, expires_at, approval_json) VALUES ('apr-'||?, ?, ?, ?, ?, X'7B7D')`,
			intent.id, intent.id, intent.status, kernel.FormatTime(now), kernel.FormatTime(now.Add(time.Hour)))
	}
	mustTx(t, db, func(ctx context.Context, tx *store.Tx) error {
		return tx.WithdrawSuperseded(ctx, "sit-1", tenant, 3, now, sources.NewVirtual(now))
	})
	for approval, want := range map[string]string{"apr-int-old": "denied", "apr-int-current": "pending", "apr-int-decided": "approved"} {
		if got := scalar[string](t, db, "SELECT status FROM approvals WHERE approval_id = ?", approval); got != want {
			t.Errorf("%s status = %s, want %s", approval, got, want)
		}
	}
	if rows := scalar[int](t, db, "SELECT COUNT(*) FROM notifications WHERE event_id LIKE 'approval.withdrawn:%'"); rows != 1 {
		t.Fatalf("approval.withdrawn notifications = %d, want 1", rows)
	}
}
