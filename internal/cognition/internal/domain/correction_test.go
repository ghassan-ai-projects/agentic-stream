package domain

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func correctionSnapshot() map[string]any {
	return map[string]any{
		"situation_id": "s", "situation_version": 3, "situation_type": "test", "tenant_id": "tenant",
		"entity": map[string]any{"type": "motor", "id": "m1"}, "phase": "watch", "severity": 10,
		"completeness": "corrected", "event_horizon": "2026-01-01T00:00:00.000000000Z",
		"spec_digest": "sha256:" + strings.Repeat("0", 64), "facts": map[string]any{},
	}
}

func correctedVersion() situations.Version {
	return situations.Version{SituationID: "s", Version: 3, PreviousVersion: 2, Completeness: "corrected"}
}

func TestOnlyACorrectedVersionUnderTheReconsiderPolicyIsReconsidered(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*situations.Version)
		policy string
		want   bool
	}{
		{"corrected under correct_and_reconsider", func(*situations.Version) {}, "correct_and_reconsider", true},
		{"corrected under correct", func(*situations.Version) {}, "correct", false},
		{"corrected under quarantine", func(*situations.Version) {}, "quarantine", false},
		{"no late policy", func(*situations.Version) {}, "", false},
		{"not corrected", func(v *situations.Version) { v.Completeness = "on_time" }, "correct_and_reconsider", false},
		{"first version has nothing to supersede", func(v *situations.Version) { v.PreviousVersion = 0 }, "correct_and_reconsider", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := correctedVersion()
			tc.mutate(&v)
			if got := ShouldReconsider(v, tc.policy); got != tc.want {
				t.Fatalf("ShouldReconsider = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReconsiderationIdentityDerivesFromSituationSupersededVersionAndCommand(t *testing.T) {
	t.Parallel()
	command := InvalidatedCommand{CommandID: "c1"}
	r := NewReconsideration(correctedVersion(), command)
	if r.ID == "" || r.TriggerID == "" || r.SchedulerItemID == "" || r.ID == r.TriggerID {
		t.Fatalf("identity = %+v", r)
	}
	if again := NewReconsideration(correctedVersion(), command); again.ID != r.ID || again.TriggerID != r.TriggerID || again.SchedulerItemID != r.SchedulerItemID {
		t.Fatalf("identity changed between identical inputs: %+v vs %+v", again, r)
	}
	otherVersion := correctedVersion()
	otherVersion.PreviousVersion = 1
	for name, other := range map[string]Reconsideration{
		"command":            NewReconsideration(correctedVersion(), InvalidatedCommand{CommandID: "c2"}),
		"superseded version": NewReconsideration(otherVersion, command),
	} {
		if other.ID == r.ID || other.TriggerID == r.TriggerID || other.SchedulerItemID == r.SchedulerItemID {
			t.Errorf("identity ignores the %s", name)
		}
	}
	laterCorrection := correctedVersion()
	laterCorrection.Version = 9
	if NewReconsideration(laterCorrection, command).ID != r.ID {
		t.Error("identity depends on the correction version: a re-delivered correction would be admitted twice")
	}
}

func TestReconsiderationEvidenceIsCanonicalAndCarriesTheCorrection(t *testing.T) {
	t.Parallel()
	r := NewReconsideration(correctedVersion(), priorFixture())
	first, err := r.EvidenceJSON(map[string]any{"corrected": true, "b": 1.0})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.EvidenceJSON(map[string]any{"b": 1.0, "corrected": true})
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("evidence bytes differ for equal corrections: %v", err)
	}
	for _, want := range []string{`"reason":"prior_action_invalidated"`, `"superseded_version":2`, `"correction_version":3`, `"invalidated_command_id":"c"`, `"correction":{"b":1,"corrected":true}`} {
		if !strings.Contains(string(first), want) {
			t.Errorf("evidence lacks %s: %s", want, first)
		}
	}
}

func TestDecodeCorrectionRefusesUnreadableSnapshots(t *testing.T) {
	t.Parallel()
	valid, err := canonicaljson.Marshal(correctionSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeCorrection(valid); err != nil || got["situation_id"] != "s" {
		t.Fatalf("valid snapshot: %v, %v", got, err)
	}
	if _, err := DecodeCorrection([]byte(`{"invalid":true}`)); err == nil || !strings.Contains(err.Error(), "decode correction snapshot") {
		t.Fatalf("schema-invalid snapshot: err = %v", err)
	}
	if _, err := DecodeCorrection(contractstest.AmbiguousKeyJSON(valid, "phase")); !errors.Is(err, contractsv1.ErrDocumentJSON) {
		t.Fatalf("ambiguous snapshot bytes: err = %v, want ErrDocumentJSON", err)
	}
}

func TestCorrectionMustMatchItsPersistedDigest(t *testing.T) {
	t.Parallel()
	correction := correctionSnapshot()
	text, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, correction)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := canonicaljson.DecodeDigest(text)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := MatchCorrectionDigest(correction, persisted); err != nil || !bytes.Equal(got, persisted) {
		t.Fatalf("matching digest: %x, %v", got, err)
	}
	tampered := correctionSnapshot()
	tampered["severity"] = 99
	for name, tc := range map[string]struct {
		correction map[string]any
		digest     []byte
	}{
		"a tampered correction": {tampered, persisted},
		"a short digest":        {correction, persisted[:31]},
		"no digest":             {correction, nil},
	} {
		if _, err := MatchCorrectionDigest(tc.correction, tc.digest); err == nil || err.Error() != "correction snapshot digest mismatch" {
			t.Errorf("%s: err = %v, want a digest mismatch", name, err)
		}
	}
}

func TestReconsiderationIsAdmittedIntoTheDeepLaneWithItsOwnEvidence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	r := NewReconsideration(correctedVersion(), priorFixture())
	eval := ReconsiderationEvaluation(r, []byte(`{"x":1}`), "sha256:digest", now)
	if eval.Outcome != "admitted" || !eval.Admitted() || eval.TriggerName != ReconsiderationTrigger || eval.Lane != spec.LaneDeep || eval.Score != 100 || eval.SituationVersion != 3 {
		t.Fatalf("evaluation = %+v", eval)
	}
	if string(eval.DeltaJSON) != `{"x":1}` || eval.PolicySHA256 != "sha256:digest" || !eval.EvaluatedAt.Equal(now) || eval.TriggerID != r.TriggerID {
		t.Fatalf("evaluation evidence = %+v", eval)
	}
	item := ReconsiderationItem(r, now)
	if item.SchedulerItemID != r.SchedulerItemID || item.TriggerID != r.TriggerID || item.Kind != "reconsider" || item.Lane != spec.LaneDeep || item.Priority != 100 || item.Status != "pending" || !item.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("item = %+v", item)
	}
}

func TestRejectedReconsiderationExplainsWhyItWasRefused(t *testing.T) {
	t.Parallel()
	r := NewReconsideration(correctedVersion(), priorFixture())
	eval := RejectedReconsiderationEvaluation(r, errors.New("executed command is empty"), "sha256:digest", time.Now())
	if eval.Outcome != "rejected" || eval.Admitted() || string(eval.DeltaJSON) != "{}" {
		t.Fatalf("evaluation = %+v, want a rejected evaluation with an empty delta", eval)
	}
	if len(eval.Reasons) != 1 || eval.Reasons[0] != "prior documents unavailable: executed command is empty" {
		t.Fatalf("reasons = %v", eval.Reasons)
	}
}

func TestReconsiderationRequiresASpecDigest(t *testing.T) {
	t.Parallel()
	if err := RequirePolicyDigest("sha256:x"); err != nil {
		t.Fatal(err)
	}
	if err := RequirePolicyDigest(""); err == nil || err.Error() != "compiled spec has no digest" {
		t.Fatalf("err = %v", err)
	}
}
