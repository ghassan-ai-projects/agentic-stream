package domain

import (
	"bytes"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func TestSchedulingTimingAndReplacementRules(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	item := episodeledger.SchedulerItem{}
	if err := ApplyTiming(&item, spec.Trigger{Debounce: "2s"}, now); err != nil {
		t.Fatal(err)
	}
	if !item.ExpiresAt.Equal(now.Add(15*time.Minute)) || !item.NotBefore.Equal(now.Add(2*time.Second)) {
		t.Fatalf("timing = %+v", item)
	}
	ApplyCooldown(&item, now, 5*time.Second)
	if !item.NotBefore.Equal(now.Add(5 * time.Second)) {
		t.Fatalf("cooldown = %v", item.NotBefore)
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
	r := NewReconsideration(v, InvalidatedCommand{CommandID: "c", ProviderJSON: []byte(`{"accepted":true}`), ObservedJSON: []byte(`{"state":"off"}`), OutcomeSHA: make([]byte, 32)})
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
	if _, _, err := DecodeCorrection([]byte(`{"invalid":true}`)); err == nil {
		t.Fatal("invalid correction schema admitted")
	}
	if _, err := MatchCorrectionDigest("sha256:0000000000000000000000000000000000000000000000000000000000000000", make([]byte, 31)); err == nil {
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
