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

type explanation struct {
	outcome    string
	wantReason string
}

func (h *harness) requireExplained(v situations.Version, want explanation) {
	h.t.Helper()
	record := h.evaluation(v)
	if record.Outcome != want.outcome {
		h.t.Fatalf("outcome of %s v%d = %q, want %q", v.SituationID, v.Version, record.Outcome, want.outcome)
	}
	if !slices.ContainsFunc(record.Reasons, func(reason string) bool { return strings.Contains(reason, want.wantReason) }) {
		h.t.Fatalf("reasons of %s v%d = %v, want one containing %q", v.SituationID, v.Version, record.Reasons, want.wantReason)
	}
}

func TestEveryCognitiveOpportunityIsExplainableFromDurableRecords(t *testing.T) {
	t.Parallel()
	t.Run("ignored", func(t *testing.T) {
		t.Parallel()
		h := newTriggerHarness(t, fastTrigger())
		v := candidateVersion("sit-1", 1, 5)
		h.process(v)
		h.requireExplained(v, explanation{"ignored", "trigger condition false"})
	})
	t.Run("admitted", func(t *testing.T) {
		t.Parallel()
		h := newTriggerHarness(t, fastTrigger())
		v := candidateVersion("sit-1", 1, 15)
		h.process(v)
		h.requireExplained(v, explanation{"admitted", "meets threshold"})
		if got := h.itemStatuses("sit-1")[1]; got != "pending" {
			t.Fatalf("queue item = %q, want pending", got)
		}
	})
	t.Run("deferred", func(t *testing.T) {
		t.Parallel()
		h := newTriggerHarness(t, fastTrigger())
		h.fillQueue(100)
		v := candidateVersion("sit-1", 1, 15)
		h.process(v)
		h.requireExplained(v, explanation{"deferred", "global capacity exhausted"})
		if got := h.itemCount(v); got != 0 {
			t.Fatalf("scheduler items = %d, want none for a deferred opportunity", got)
		}
	})
	t.Run("coalesced", func(t *testing.T) {
		t.Parallel()
		h := newTriggerHarness(t, fastTrigger())
		first := candidateVersion("sit-1", 1, 15)
		h.process(first)
		h.process(candidateVersion("sit-1", 2, 20))
		h.requireExplained(first, explanation{"admitted", "meets threshold"})
		if got := h.itemStatuses("sit-1"); got[1] != "coalesced" || got[2] != "pending" {
			t.Fatalf("queue items = %v, want version 1 coalesced and version 2 pending", got)
		}
		if rows := scalar[int](h, "SELECT COUNT(*) FROM notifications WHERE event_id LIKE 'situation.superseded:sit-1:1:2:%'"); rows != 1 {
			t.Fatalf("supersession notifications = %d, want 1", rows)
		}
	})
	t.Run("rejected", func(t *testing.T) {
		t.Parallel()
		f := newReconsiderationFixture(t, immediatePredecessor)
		f.insertExecutedCommand("cmd-bad", "dec-bad", "epi-bad", "int-bad", 1, "approved")
		f.exec("UPDATE commands SET command_json = X'7B' WHERE command_id = 'cmd-bad'")
		if err := f.process(); err != nil {
			t.Fatal(err)
		}
		reason := scalarString(f, "SELECT reasons_json FROM trigger_evaluations WHERE outcome = 'rejected'")
		if !strings.Contains(reason, "prior documents unavailable") {
			t.Fatalf("rejected reconsideration reasons = %s, want the unavailable prior documents", reason)
		}
	})
	t.Run("refused by cost control", func(t *testing.T) {
		t.Parallel()
		h, v, itemID := admittedItem(t)
		err := h.db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return RecordCostRejectionReason(t.Context(), store.Join(tx), itemID, errors.New("budget exhausted"))
		})
		if err != nil {
			t.Fatal(err)
		}
		h.requireExplained(v, explanation{"admitted", "rejected by cost control: budget exhausted"})
	})
	t.Run("expired", func(t *testing.T) {
		t.Parallel()
		h, v, itemID := admittedItem(t)
		err := h.db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return RecordSchedulerExpiryReason(t.Context(), store.Join(tx), itemID, "unreadable expires_at")
		})
		if err != nil {
			t.Fatal(err)
		}
		h.requireExplained(v, explanation{"admitted", "scheduler item expired: unreadable expires_at"})
	})
}
