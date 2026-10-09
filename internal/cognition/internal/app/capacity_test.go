package app

import "testing"

func TestQueueJustBelowCapacityStillAdmits(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	h.fillQueue(99)
	v := candidateVersion("sit-cap", 1, 15)
	h.process(v)
	if record := h.evaluation(v); record.Outcome != "admitted" || h.itemCount(v) != 1 {
		t.Fatalf("evaluation = %+v, items %d; want admitted with one item", record, h.itemCount(v))
	}
}

func TestFullQueueStillAdmitsAVersionThatReplacesItsOwnPendingItem(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	h.fillQueue(99)
	h.process(candidateVersion("sit-1", 1, 15))
	second := candidateVersion("sit-1", 2, 20)
	h.process(second)
	if record := h.evaluation(second); record.Outcome != "admitted" {
		t.Fatalf("evaluation = %+v, want admitted: it replaces the pending item of the same trigger", record)
	}
	if got := h.itemStatuses("sit-1"); got[1] != "coalesced" || got[2] != "pending" {
		t.Fatalf("item statuses = %v, want version 1 coalesced and version 2 pending", got)
	}
}
