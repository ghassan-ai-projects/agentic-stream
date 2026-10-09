package domain

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestTimingKeepsAnItemUsefulForExpiresAfterOnceItMayStart(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	latest := now.Add(-time.Minute)
	for name, test := range map[string]struct {
		trigger       spec.Trigger
		latest        *time.Time
		expiresAfter  time.Duration
		wantNotBefore time.Duration
	}{
		"no delay":                    {trigger: spec.Trigger{}, expiresAfter: 15 * time.Minute},
		"debounce":                    {trigger: spec.Trigger{Debounce: "2m"}, expiresAfter: 17 * time.Minute, wantNotBefore: 2 * time.Minute},
		"cooldown above expiry":       {trigger: spec.Trigger{Cooldown: "30m"}, latest: &latest, expiresAfter: 44 * time.Minute, wantNotBefore: 29 * time.Minute},
		"debounce above expiry":       {trigger: spec.Trigger{Debounce: "20m", ExpiresAfter: "10m"}, expiresAfter: 30 * time.Minute, wantNotBefore: 20 * time.Minute},
		"cooldown without history":    {trigger: spec.Trigger{Cooldown: "30m"}, expiresAfter: 15 * time.Minute},
		"cooldown already elapsed":    {trigger: spec.Trigger{Cooldown: "30s"}, latest: &latest, expiresAfter: 15 * time.Minute, wantNotBefore: -30 * time.Second},
		"debounce wins over cooldown": {trigger: spec.Trigger{Debounce: "5m", Cooldown: "2m"}, latest: &latest, expiresAfter: 20 * time.Minute, wantNotBefore: 5 * time.Minute},
		"cooldown wins over debounce": {trigger: spec.Trigger{Debounce: "1m", Cooldown: "10m"}, latest: &latest, expiresAfter: 24 * time.Minute, wantNotBefore: 9 * time.Minute},
		"explicit expiry":             {trigger: spec.Trigger{ExpiresAfter: "1h"}, expiresAfter: time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var item episodeledger.SchedulerItem
			if err := ApplyTiming(&item, test.trigger, now, test.latest); err != nil {
				t.Fatal(err)
			}
			if got := item.ExpiresAt.Sub(now); got != test.expiresAfter {
				t.Errorf("expires %v after now, want %v", got, test.expiresAfter)
			}
			if (item.NotBefore == nil) != (test.wantNotBefore == 0) || item.NotBefore != nil && item.NotBefore.Sub(now) != test.wantNotBefore {
				t.Errorf("not before = %v, want %v after now", item.NotBefore, test.wantNotBefore)
			}
		})
	}
}

func TestTimingRefusesAnUnreadableDuration(t *testing.T) {
	t.Parallel()
	for field, trigger := range map[string]spec.Trigger{
		"expiresAfter": {ExpiresAfter: "x"},
		"debounce":     {Debounce: "x"},
		"cooldown":     {Cooldown: "x"},
	} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			var item episodeledger.SchedulerItem
			err := ApplyTiming(&item, trigger, time.Now(), nil)
			if err == nil || !strings.Contains(err.Error(), "parse "+field+":") {
				t.Fatalf("error = %v, want one naming %s", err, field)
			}
		})
	}
}

func TestParseOptionalDurationDefaultsOnlyWhenEmpty(t *testing.T) {
	t.Parallel()
	if got, err := ParseOptionalDuration("", 7*time.Second); err != nil || got != 7*time.Second {
		t.Fatalf("empty = %v, %v; want the default 7s", got, err)
	}
	if got, err := ParseOptionalDuration("2d", 7*time.Second); err != nil || got != 48*time.Hour {
		t.Fatalf("2d = %v, %v; want 48h", got, err)
	}
	if _, err := ParseOptionalDuration("soon", 7*time.Second); err == nil || !strings.Contains(err.Error(), `parse duration "soon"`) {
		t.Fatalf("soon: err = %v, want one naming the text", err)
	}
}

func TestGlobalCapacityIsExhaustedOnlyWithoutAReplaceableItem(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pending, sameTrigger int
		want                 bool
	}{
		{0, 0, false},
		{99, 0, false},
		{100, 0, true},
		{250, 0, true},
		{100, 1, false},
		{250, 3, false},
	}
	for _, tc := range tests {
		if got := CapacityExhausted(tc.pending, tc.sameTrigger); got != tc.want {
			t.Errorf("CapacityExhausted(%d, %d) = %v, want %v", tc.pending, tc.sameTrigger, got, tc.want)
		}
	}
}

func TestOnlyAnOlderVersionIsSupersededByANewerOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		replacement, old int
		want             bool
	}{{2, 1, true}, {2, 2, false}, {1, 2, false}} {
		if got := Supersedes(tc.replacement, tc.old); got != tc.want {
			t.Errorf("Supersedes(%d, %d) = %v, want %v", tc.replacement, tc.old, got, tc.want)
		}
	}
}

func TestSchedulerDedupeKeyIdentifiesSituationVersionAndTrigger(t *testing.T) {
	t.Parallel()
	key := SchedulerDedupeKey("s", 2, "t")
	if !bytes.Equal(key, SchedulerDedupeKey("s", 2, "t")) || len(key) != 32 {
		t.Fatalf("key = %x, want a stable 32-byte digest", key)
	}
	for name, other := range map[string][]byte{
		"version":   SchedulerDedupeKey("s", 3, "t"),
		"situation": SchedulerDedupeKey("s2", 2, "t"),
		"trigger":   SchedulerDedupeKey("s", 2, "t2"),
	} {
		if bytes.Equal(key, other) {
			t.Errorf("key ignores the %s", name)
		}
	}
}

func TestExplanationsAppendToTheEvaluationsReasons(t *testing.T) {
	t.Parallel()
	deferred := Evaluation{Outcome: "admitted", Reasons: []string{"score 9.00 meets threshold 5.00"}}
	Defer(&deferred)
	if deferred.Outcome != "deferred" || !slices.Equal(deferred.Reasons, []string{"score 9.00 meets threshold 5.00", "global capacity exhausted"}) {
		t.Fatalf("deferred = %+v", deferred)
	}
	if got := RecordCostRefusal([]string{"a"}, "budget exhausted"); !slices.Equal(got, []string{"a", "episode admission rejected by cost control: budget exhausted"}) {
		t.Fatalf("cost refusal reasons = %v", got)
	}
	if got := RecordSchedulerExpiry(nil, "unreadable expires_at"); !slices.Equal(got, []string{"scheduler item expired: unreadable expires_at"}) {
		t.Fatalf("expiry reasons = %v", got)
	}
}

func TestFindTriggerNamesTheTriggerItCouldNotFind(t *testing.T) {
	t.Parallel()
	compiled := &spec.CompiledSpec{Cognition: spec.Cognition{Triggers: []spec.Trigger{{Name: "a", Lane: "fast"}, {Name: "b", Lane: "deep"}}}}
	if got, err := FindTrigger(compiled, "b"); err != nil || got.Lane != "deep" {
		t.Fatalf("FindTrigger(b) = %+v, %v", got, err)
	}
	if _, err := FindTrigger(compiled, "c"); err == nil || err.Error() != `trigger "c" not found` {
		t.Fatalf("FindTrigger(c) error = %v", err)
	}
}

func TestSchedulerItemBindsAnAdmittedEvaluationToItsQueueRecord(t *testing.T) {
	t.Parallel()
	eval := Evaluation{TriggerID: "trg_abc", SituationID: "sit", SituationVersion: 4, Lane: "deep", Score: 42}
	item := NewSchedulerItem(eval)
	want := episodeledger.SchedulerItem{SchedulerItemID: "sch_abc", Kind: episodeledger.KindStandard, TriggerID: "trg_abc", SituationID: "sit", SituationVersion: 4, Lane: "deep", Priority: 42, Status: "pending"}
	if item != want {
		t.Fatalf("item = %+v, want %+v", item, want)
	}
}

func TestReconsiderationExpiresAfterTheDefaultWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	if got := ReconsiderationExpiry(now); !got.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("expiry = %s, want 15 minutes after %s", got, now)
	}
}
