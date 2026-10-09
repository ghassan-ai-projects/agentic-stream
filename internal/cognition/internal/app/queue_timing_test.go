package app

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestDebounceAndCooldownDecideWhenAnAdmittedItemMayStart(t *testing.T) {
	t.Parallel()
	timed := func(debounce, cooldown string) spec.Trigger {
		tr := fastTrigger()
		tr.Debounce, tr.Cooldown = debounce, cooldown
		return tr
	}
	tests := []struct {
		name     string
		trigger  spec.Trigger
		firstRan bool
		second   time.Duration
		want     time.Duration
	}{
		{"no timing declared", timed("", ""), true, 2 * time.Minute, 0},
		{"debounce delays from the evaluation", timed("5m", ""), true, 2 * time.Minute, 7 * time.Minute},
		{"cooldown delays from the previous admission that ran", timed("", "10m"), true, 2 * time.Minute, 10 * time.Minute},
		{"the later of debounce and cooldown wins", timed("3m", "10m"), true, 2 * time.Minute, 10 * time.Minute},
		{"debounce wins when it ends later", timed("20m", "10m"), true, 2 * time.Minute, 22 * time.Minute},
		{"a superseded admission that never ran starts no cooldown", timed("", "10m"), false, 2 * time.Minute, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTriggerHarness(t, tc.trigger)
			h.process(candidateVersion("sit-1", 1, 15))
			if tc.firstRan {
				h.exec("UPDATE scheduler_items SET status = 'admitted' WHERE situation_version = 1")
			}
			h.clock.Advance(tc.second)
			second := candidateVersion("sit-1", 2, 20)
			h.process(second)
			got := h.notBefore(second)
			if tc.want == 0 {
				if got.Valid {
					t.Fatalf("not_before = %s, want none", got.String)
				}
				return
			}
			if want := kernel.FormatTime(base.Add(tc.want)); got != (sql.NullString{String: want, Valid: true}) {
				t.Fatalf("not_before = %s, want %s", got.String, want)
			}
		})
	}
}

func TestFirstAdmissionHasNoCooldownToWaitFor(t *testing.T) {
	t.Parallel()
	trigger := fastTrigger()
	trigger.Cooldown = "10m"
	h := newTriggerHarness(t, trigger)
	first := candidateVersion("sit-1", 1, 15)
	h.process(first)
	if got := h.notBefore(first); got.Valid {
		t.Fatalf("not_before = %s, want none: nothing was admitted before", got.String)
	}
}

func TestUnreadableStoredTimesRefuseTheNextVersionOfThatSituationOnly(t *testing.T) {
	t.Parallel()
	for name, corrupt := range map[string]string{
		"event horizon of the last reasoned version": "UPDATE situation_versions SET event_horizon = 'not a time' WHERE situation_id = 'sit-1'",
		"watermark of the last reasoned version":     "UPDATE situation_versions SET watermark = 'not a time' WHERE situation_id = 'sit-1'",
		"evaluation time of the last admission":      "UPDATE trigger_evaluations SET evaluated_at = 'not a time' WHERE situation_id = 'sit-1'",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			trigger := fastTrigger()
			trigger.Cooldown = "10m"
			h := newTriggerHarness(t, trigger)
			h.process(candidateVersion("sit-1", 1, 15))
			h.exec("UPDATE scheduler_items SET status = 'admitted' WHERE situation_id = 'sit-1'")
			h.exec(corrupt)
			other := candidateVersion("sit-2", 1, 15)
			h.process(other)
			if err := h.tryProcess(candidateVersion("sit-1", 2, 20)); err == nil {
				t.Fatal("a version whose predecessor has an unreadable time was processed")
			}
			if record := h.evaluation(other); record.Outcome != "admitted" {
				t.Fatalf("the other situation's evaluation = %s, want admitted", record.Outcome)
			}
		})
	}
}
