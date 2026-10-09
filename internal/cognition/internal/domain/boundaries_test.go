package domain

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestSchedulingTimingAndReplacementRules(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	item := episodeledger.SchedulerItem{}
	if err := ApplyTiming(&item, spec.Trigger{Debounce: "2s"}, now, nil); err != nil {
		t.Fatal(err)
	}
	if !item.ExpiresAt.Equal(now.Add(15*time.Minute+2*time.Second)) || !item.NotBefore.Equal(now.Add(2*time.Second)) {
		t.Fatalf("timing = %+v", item)
	}
	if !CapacityExhausted(100, 0) || CapacityExhausted(100, 1) || !Supersedes(2, 1) || Supersedes(2, 2) {
		t.Fatal("capacity or replacement decision changed")
	}
	if bytes.Equal(SchedulerDedupeKey("s", 2, "t"), SchedulerDedupeKey("s", 3, "t")) {
		t.Fatal("queue identity ignores version")
	}
	eval := Evaluation{}
	Defer(&eval)
	if eval.Outcome != "deferred" || len(RecordCostRefusal(eval.Reasons, "budget")) != 2 {
		t.Fatal("durable refusal vocabulary changed")
	}
}

func TestReconsiderationEvidenceAndDigestRefusal(t *testing.T) {
	t.Parallel()
	v := situations.Version{SituationID: "s", Version: 3, PreviousVersion: 2, Completeness: "corrected"}
	if !ShouldReconsider(v, "correct_and_reconsider") || ShouldReconsider(v, "quarantine") {
		t.Fatal("correction eligibility changed")
	}
	r := NewReconsideration(v, priorFixture())
	if r.ID == "" || r.TriggerID == "" || r.SchedulerItemID == "" {
		t.Fatal("correction identity missing")
	}
	first, err := r.EvidenceJSON(map[string]any{"corrected": true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.EvidenceJSON(map[string]any{"corrected": true})
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("correction bytes changed: %v", err)
	}
	if _, err := DecodeCorrection([]byte(`{"invalid":true}`)); err == nil {
		t.Fatal("invalid correction schema admitted")
	}
	if _, err := DecodeCorrection(contractstest.AmbiguousKeyJSON([]byte(`{"phase":"warning"}`), "phase")); !errors.Is(err, contractsv1.ErrDocumentJSON) {
		t.Fatalf("ambiguous correction bytes err = %v", err)
	}
	if _, err := MatchCorrectionDigest(map[string]any{}, make([]byte, 31)); err == nil {
		t.Fatal("incomplete persisted digest admitted")
	}
}

func TestDeltaUsesPreviousFactsAndConditionDefaults(t *testing.T) {
	t.Parallel()
	trigger := spec.Trigger{Name: "change", When: "delta.facts_changed", Score: "5.0", Threshold: 5}
	rules, err := NewRules(&spec.CompiledSpec{Cognition: spec.Cognition{Triggers: []spec.Trigger{trigger}}})
	if err != nil {
		t.Fatal(err)
	}
	previous := situations.Version{Phase: "watch", Facts: map[string]any{"temperature": 10.0}}
	current := situations.Version{SituationID: "s", Version: 2, Phase: "watch", Facts: map[string]any{"temperature": 20.0}}
	eval, err := rules.Evaluate(EvaluationInput{Trigger: trigger, Current: current, Previous: &previous, Now: time.Now(), DeploymentID: "dep"})
	if err != nil || eval.Outcome != "admitted" {
		t.Fatalf("changed facts = %+v %v", eval, err)
	}
	eval, err = rules.Evaluate(EvaluationInput{Trigger: trigger, Current: current, Previous: &current, Now: time.Now(), DeploymentID: "dep"})
	if err != nil || eval.Outcome != "ignored" {
		t.Fatalf("unchanged facts = %+v %v", eval, err)
	}
}

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
	for _, trigger := range []spec.Trigger{{ExpiresAfter: "x"}, {Debounce: "x"}, {Cooldown: "x"}} {
		var item episodeledger.SchedulerItem
		if err := ApplyTiming(&item, trigger, time.Now(), nil); err == nil {
			t.Errorf("%+v accepted", trigger)
		}
	}
}
