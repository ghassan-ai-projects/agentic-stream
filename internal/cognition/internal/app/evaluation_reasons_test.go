package app

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func admittedItem(t *testing.T) (*harness, situations.Version, string) {
	t.Helper()
	h := newTriggerHarness(t, fastTrigger())
	v := candidateVersion("sit-1", 1, 15)
	h.process(v)
	return h, v, scalar[string](h, "SELECT scheduler_item_id FROM scheduler_items WHERE situation_id = 'sit-1'")
}

func TestCostRefusalIsAddedToTheEvaluationInTheCallersTransactionOnly(t *testing.T) {
	t.Parallel()
	h, v, itemID := admittedItem(t)
	admission := h.evaluation(v).Reasons
	refusal := errors.New("budget exhausted")
	record := func(tx *sql.Tx) error { return RecordCostRejectionReason(t.Context(), store.Join(tx), itemID, refusal) }
	rollback := errors.New("queue transition failed")
	err := h.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := record(tx); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) || !slices.Equal(h.evaluation(v).Reasons, admission) {
		t.Fatalf("after rollback: err %v reasons %v, want the admission reasons only", err, h.evaluation(v).Reasons)
	}
	if err := h.db.WithTx(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	reasons := h.evaluation(v).Reasons
	if len(reasons) != len(admission)+1 || reasons[len(reasons)-1] != "episode admission rejected by cost control: budget exhausted" {
		t.Fatalf("reasons = %v, want the cost refusal appended to %v", reasons, admission)
	}
	if status := h.itemStatuses("sit-1")[1]; status != "pending" {
		t.Fatalf("item status = %s: the audit must leave queue state to its owner", status)
	}
}

func TestSchedulerExpiryIsAddedToTheEvaluationWithoutTouchingTheQueue(t *testing.T) {
	t.Parallel()
	h, v, itemID := admittedItem(t)
	err := h.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return RecordSchedulerExpiryReason(t.Context(), store.Join(tx), itemID, "unreadable expires_at")
	})
	if err != nil {
		t.Fatal(err)
	}
	reasons := h.evaluation(v).Reasons
	if reasons[len(reasons)-1] != "scheduler item expired: unreadable expires_at" {
		t.Fatalf("reasons = %v, want the expiry appended", reasons)
	}
	if status := h.itemStatuses("sit-1")[1]; status != "pending" {
		t.Fatalf("item status = %s: the audit must leave queue state to its owner", status)
	}
}

func TestReasonOfAnItemWithoutAQueueRowIsRefused(t *testing.T) {
	t.Parallel()
	h, v, _ := admittedItem(t)
	before := h.evaluation(v).Reasons
	err := h.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return RecordSchedulerExpiryReason(t.Context(), store.Join(tx), "sch-unknown", "gone")
	})
	if err == nil || !strings.Contains(err.Error(), "load trigger evaluation reasons") || !slices.Equal(h.evaluation(v).Reasons, before) {
		t.Fatalf("error = %v, want a refusal that leaves the reasons alone", err)
	}
}

func TestReasonRecordingRefusesAMissingTransactionOrReason(t *testing.T) {
	t.Parallel()
	h, _, itemID := admittedItem(t)
	err := h.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		joined := store.Join(tx)
		checks := map[string]error{
			"cost refusal without a transaction": RecordCostRejectionReason(t.Context(), store.Join(nil), itemID, errors.New("x")),
			"cost refusal without a reason":      RecordCostRejectionReason(t.Context(), joined, itemID, nil),
			"expiry without a transaction":       RecordSchedulerExpiryReason(t.Context(), store.Join(nil), itemID, "x"),
			"expiry without a reason":            RecordSchedulerExpiryReason(t.Context(), joined, itemID, ""),
		}
		for name, got := range checks {
			if got == nil || !strings.Contains(got.Error(), "requires transaction and reason") {
				t.Errorf("%s: err = %v, want a missing-input refusal", name, got)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
