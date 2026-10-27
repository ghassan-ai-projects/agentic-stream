package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

var driftKey = Key{DiagnosisCode: "sensor_drift", KindDigest: "sha256:pump", Intent: "schedule_maintenance", Preset: "sensor_drift"}

func driftCase(id, verdict string, vibration float64) KeyedCase {
	return KeyedCase{Key: driftKey, EpisodeID: id, Verdict: verdict, Summary: "probe drift",
		Features: map[string]any{"vibration_mean_15m": vibration, "vibration_mean_24h": 3.0, "current_mean_15m": 30.0}}
}

func TestEligibilityNeedsEnoughUncontradictedEvidenceAndNoOpenProposal(t *testing.T) {
	t.Parallel()
	three := []KeyedCase{driftCase("e1", CaseVerified, 4.0), driftCase("e2", CaseVerified, 4.2), driftCase("e3", CaseVerified, 4.4)}
	otherAnswer := driftCase("e4", CaseVerified, 4.1)
	otherAnswer.Preset = "bearing_wear"
	failed := driftCase("e5", CaseNotVerified, 4.1)
	cases := []struct {
		name     string
		cases    []KeyedCase
		history  KeyHistory
		ruled    bool
		eligible bool
	}{
		{"three verified", three, KeyHistory{}, false, true},
		{"two verified", three[:2], KeyHistory{}, false, false},
		{"another answer verified", append(three, otherAnswer), KeyHistory{}, false, false},
		{"same answer not verified", append(three, failed), KeyHistory{}, false, false},
		{"pending proposal", three, KeyHistory{Pending: true}, false, false},
		{"rejected without newer evidence", three, KeyHistory{Rejected: true, RejectedEvidence: 3}, false, false},
		{"rejected before newer evidence", three, KeyHistory{Rejected: true, RejectedEvidence: 2}, false, true},
		{"already ruled", three, KeyHistory{}, true, false},
	}
	for _, tc := range cases {
		if got := Eligible(driftKey, tc.cases, tc.history, DefaultDraftAfter, tc.ruled); got != tc.eligible {
			t.Errorf("%s: eligible = %v, want %v", tc.name, got, tc.eligible)
		}
	}
	unknown := driftKey
	unknown.DiagnosisCode = spec.UnknownDiagnosis
	if Eligible(unknown, three, KeyHistory{}, DefaultDraftAfter, false) || len(Keys(three)) != 1 {
		t.Fatal("the unknown diagnosis never becomes a rule; three cases of one key are one key")
	}
}

func driftRequest(rules *Rules) draftport.Request {
	cases := []KeyedCase{driftCase("e1", CaseVerified, 4.0), driftCase("e2", CaseVerified, 4.4)}
	return rules.DraftRequestFor("rdraft_1", "t", driftKey, cases)
}

func driftProposal(when string, cited ...string) draftport.Proposal {
	return draftport.Proposal{DiagnosisCode: driftKey.DiagnosisCode, Intent: driftKey.Intent, Preset: driftKey.Preset, When: when, CitedEpisodeIDs: cited}
}

func TestAProposalMustAnswerItsKeyCiteSentCasesAndHoldForThem(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	request := driftRequest(rules)
	if len(request.Cases) != 2 || request.SpecDigest == "" || len(request.AllowedFeatures) == 0 {
		t.Fatalf("request = %+v", request)
	}
	good := driftProposal("features.vibration_mean_15m > 3.9", "e1", "e2")
	if err := rules.CheckProposal(request, good); err != nil {
		t.Fatalf("a sound proposal was refused: %v", err)
	}
	wrongKey := good
	wrongKey.Preset = "bearing_wear"
	refusals := map[string]draftport.Proposal{
		"another key":         wrongKey,
		"an unsent case":      driftProposal("features.vibration_mean_15m > 3.9", "e1", "e9"),
		"no cited case":       driftProposal("features.vibration_mean_15m > 3.9"),
		"a condition missing": driftProposal("features.vibration_mean_15m > 4.2", "e1", "e2"),
		"a broken condition":  driftProposal("features.vibration_mean_15m >", "e1"),
	}
	for name, proposal := range refusals {
		if err := rules.CheckProposal(request, proposal); !errors.Is(err, ErrProposalRefused) {
			t.Errorf("%s: check = %v, want refused", name, err)
		}
	}
}

func TestTheRuntimeBuildsTheProposedRule(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	request := driftRequest(rules)
	rule, err := rules.RuleFromProposal(request, driftProposal("features.vibration_mean_15m > 3.9", "e1", "e2"), RuleIdentity{Version: 1, KindName: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	if rule.ID != "sensor-drift.schedule-maintenance.sensor-drift" || rule.Mode != spec.RuleModeShadow || rule.PinnedKind.Digest != "sha256:pump" ||
		rule.Envelope["vibration_mean_15m"] != (spec.Range{Min: 4.0, Max: 4.4}) || rule.Cap["current_mean_15m"].Max <= 30 {
		t.Fatalf("rule = %+v, want a shadow rule pinned to the case kind with the cases' envelope", rule)
	}
	if _, err := rules.RuleFromProposal(request, driftProposal("features.unknown_feature > 1.0", "e1"), RuleIdentity{}); !errors.Is(err, ErrProposalRefused) {
		t.Fatalf("a condition over an unknown feature = %v, want refused", err)
	}
	if rules.NextVersion(rule.ID) != 1 || rules.NextVersion("bearing-wear") != 2 {
		t.Fatal("the next version of a new rule is 1, of an existing one its version plus one")
	}
}

func TestAProposedRuleCompilesBesideTheRulesOfAnotherKindAndReplacesItsOlderVersion(t *testing.T) {
	t.Parallel()
	probeB := Key{DiagnosisCode: "bearing_wear", KindDigest: "sha256:" + strings.Repeat("ab", 32), Intent: "schedule_maintenance", Preset: "bearing_wear"}
	if id := RuleID(probeB); id != "bearing-wear.schedule-maintenance.bearing-wear.abababababab" {
		t.Fatalf("rule id = %s, want the key's kind in it", id)
	}
	rules := pumpRules(t)
	rule, err := rules.RuleFromProposal(driftRequest(rules), driftProposal("features.vibration_mean_15m > 3.9", "e1", "e2"), RuleIdentity{Version: 1, KindName: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	rule.PinnedKind.Digest = "sha256:" + strings.Repeat("b", 64)
	for _, version := range []int{1, 2} {
		rule.Version = version
		book := rules.WithRule(rule)
		document, err := spec.MarshalPlaybook(book)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := spec.CompilePlaybook(rules.Spec(), document); err != nil {
			t.Fatalf("version %d: %v, want the candidate playbook to compile", version, err)
		}
		if count := len(slices.DeleteFunc(slices.Clone(book.Rules), func(r spec.Rule) bool { return r.ID != rule.ID })); count != 1 {
			t.Fatalf("version %d: %d rules named %s, want the new version alone", version, count, rule.ID)
		}
	}
}

func TestTheReplayReportCountsWhatTheRuleWouldHaveDone(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	rule, err := rules.RuleFromProposal(driftRequest(rules), driftProposal("features.vibration_mean_15m > 3.9", "e1", "e2"), RuleIdentity{Version: 1, KindName: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	facts := map[string]any{"facts.vibration_mean_15m": 4.2, "facts.current_mean_15m": 30.0}
	agreeing := map[string]any{"diagnosis_code": "sensor_drift", "intents": []any{map[string]any{"type": "schedule_maintenance", "parameters": map[string]any{"work_order": "sensor_drift"}}}}
	versions := []ReplayVersion{{Facts: facts, Decision: agreeing}, {Facts: facts, Decision: map[string]any{"diagnosis_code": "bearing_wear"}},
		{Facts: facts}, {Facts: map[string]any{"facts.vibration_mean_15m": 3.0}}}
	report, err := rules.Replay(rule, versions, []KeyedCase{driftCase("e1", CaseVerified, 4.0), driftCase("e9", CaseNotVerified, 4.0)})
	want := ReplayReport{VersionsExamined: 4, WouldFire: 3, Decided: 2, Agree: 1, Disagree: 1, SourceCases: 2, SourceNotVerified: 1}
	if err != nil || report != want {
		t.Fatalf("report = %+v (%v), want %+v", report, err, want)
	}
}

func TestAnApprovalSignsTheProposalAndItsBinding(t *testing.T) {
	t.Parallel()
	rules := pumpRules(t)
	rule, err := rules.RuleFromProposal(driftRequest(rules), driftProposal("features.vibration_mean_15m > 3.9", "e1", "e2"), RuleIdentity{Version: 1, KindName: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	rule.AuditEvery, rule.ShadowExit = 3, &spec.ShadowExit{Agreeing: 10}
	draft := NewDraft("t", driftKey, 2)
	proposal, err := NewProposal(draft, rule, "R1", ReplayReport{}, driftProposal(rule.When, "e1"))
	if err != nil || !IsProposalID(proposal.ProposalID) || proposal.Digest == "" || NewDraft("t", driftKey, 2).DraftID != draft.DraftID {
		t.Fatalf("proposal = %+v (%v)", proposal, err)
	}
	binding := proposal.DefaultBinding("operator", "relay", true)
	approve, err := proposal.SigningBytes(binding)
	if err != nil {
		t.Fatal(err)
	}
	reject, _ := proposal.SigningBytes(proposal.DefaultBinding("operator", "relay", false))
	if string(approve) == string(reject) || !strings.Contains(string(approve), proposal.Digest) {
		t.Fatal("an approval and a rejection sign different bytes that bind the proposal digest")
	}
	approved := proposal.ApprovedRule(binding, "sha256:signed")
	if approved.Mode != spec.RuleModeShadow || approved.AuditEvery != 3 || approved.ShadowExit.Agreeing != 10 || approved.Provenance.ApprovalDigest != "sha256:signed" {
		t.Fatalf("approved rule = %+v", approved)
	}
	if !ExitMet(approved, 10, 0) || ExitMet(approved, 10, 1) || ExitMet(approved, 9, 0) {
		t.Fatal("the shadow exit needs ten agreeing and no disagreeing evaluations")
	}
}
