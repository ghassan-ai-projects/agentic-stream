package app

import "testing"

func TestSupersessionLeavesOtherSituationsAndTriggersAlone(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	h.process(candidateVersion("sit-1", 1, 15))
	h.process(candidateVersion("sit-2", 1, 15))
	h.process(candidateVersion("sit-1", 2, 20))
	if got := h.itemStatuses("sit-2"); got[1] != "pending" {
		t.Fatalf("another situation's item = %v, want it still pending", got)
	}
}

func TestReEvaluatingAVersionDoesNotAnnounceItAsSuperseded(t *testing.T) {
	t.Parallel()
	h := newTriggerHarness(t, fastTrigger())
	v := candidateVersion("sit-1", 1, 15)
	h.process(v)
	h.reprocess(v)
	if rows := scalar[int](h, "SELECT COUNT(*) FROM notifications WHERE event_id LIKE 'situation.superseded:%'"); rows != 0 {
		t.Fatalf("superseded notifications = %d, want none: no older version was replaced", rows)
	}
}
