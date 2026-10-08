package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func TestShadowScoreUsesHighestRisk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		risk, reason string
		score        ShadowScore
	}{
		{"R1", "would_approve_R1", ShadowWouldApprove},
		{"R2", "would_require_approval_r2", ShadowWouldRequireApproval},
		{"R4", "would_deny_R4", ShadowWouldDeny},
	}
	for _, test := range tests {
		t.Run(test.risk, func(t *testing.T) {
			t.Parallel()
			validated := &decisions.Result{Intents: []decisions.Intent{{RiskClass: "R0"}, {RiskClass: test.risk}, {RiskClass: "R0"}}}
			score, reason := ScoreShadowDecision(validated)
			if score != test.score || reason != test.reason {
				t.Fatalf("score=%s reason=%s", score, reason)
			}
			identity := ShadowDecisionIdentity{ShadowDecisionID: "shadow", EpisodeID: "epi", DecisionID: "dec", TenantID: "tenant", SituationID: "sit", SituationVersion: 3, PolicyEpoch: "epoch"}
			row := NewShadowDecision(identity, episodeledger.Identity{AttemptID: "attempt", Fence: 2}, []byte("decision"), []byte("digest"), score, reason)
			if row.EpisodeID != "epi" || row.Fence != 2 || row.ShadowScore != score || row.PolicyEpoch != "epoch" {
				t.Fatalf("shadow row = %#v", row)
			}
		})
	}
}

func TestShadowScoreAgreesWithTheLiveRouteForEveryRisk(t *testing.T) {
	t.Parallel()
	scores := map[contractsv1.Route]ShadowScore{
		contractsv1.RouteAutomatic: ShadowWouldApprove,
		contractsv1.RouteApproval:  ShadowWouldRequireApproval,
		contractsv1.RouteDenied:    ShadowWouldDeny,
	}
	for _, risk := range contractstest.RiskClasses() {
		for _, flagged := range []bool{false, true} {
			validated := &decisions.Result{Intents: []decisions.Intent{{RiskClass: string(risk), RequiresApproval: flagged}}}
			score, _ := ScoreShadowDecision(validated)
			if want := scores[contractsv1.RouteFor(contractsv1.RiskClass(risk), flagged)]; score != want {
				t.Fatalf("%s requires_approval=%t: score %s, want %s", risk, flagged, score, want)
			}
		}
	}
}

func TestShadowScoreHonoursRequiresApprovalOnLowRisk(t *testing.T) {
	t.Parallel()
	validated := &decisions.Result{Intents: []decisions.Intent{{RiskClass: "R1", RequiresApproval: true}}}
	score, reason := ScoreShadowDecision(validated)
	if score != ShadowWouldRequireApproval || reason != "would_require_approval_r1" {
		t.Fatalf("score=%s reason=%s", score, reason)
	}
}

func TestStrictestIntentFollowsTheRiskOrder(t *testing.T) {
	t.Parallel()
	classes := contractstest.RiskClasses()
	for index, want := range classes {
		validated := &decisions.Result{Intents: []decisions.Intent{{RiskClass: string(classes[0])}, {RiskClass: string(want)}, {RiskClass: string(classes[0])}}}
		if got := strictestIntent(validated).RiskClass; got != string(want) {
			t.Fatalf("index %d: strictest = %s, want %s", index, got, want)
		}
	}
}

func TestShadowScoreOfTwoIntentsIsTheStricterLiveRoute(t *testing.T) {
	t.Parallel()
	scores := map[contractsv1.Route]ShadowScore{
		contractsv1.RouteAutomatic: ShadowWouldApprove,
		contractsv1.RouteApproval:  ShadowWouldRequireApproval,
		contractsv1.RouteDenied:    ShadowWouldDeny,
	}
	strictness := map[contractsv1.Route]int{contractsv1.RouteAutomatic: 0, contractsv1.RouteApproval: 1, contractsv1.RouteDenied: 2}
	var intents []decisions.Intent
	for _, risk := range contractstest.RiskClasses() {
		intents = append(intents, decisions.Intent{RiskClass: risk}, decisions.Intent{RiskClass: risk, RequiresApproval: true})
	}
	for _, first := range intents {
		for _, second := range intents {
			live := func(intent decisions.Intent) contractsv1.Route {
				return contractsv1.RouteFor(contractsv1.RiskClass(intent.RiskClass), intent.RequiresApproval)
			}
			want := live(first)
			if strictness[live(second)] > strictness[want] {
				want = live(second)
			}
			score, _ := ScoreShadowDecision(&decisions.Result{Intents: []decisions.Intent{first, second}})
			if score != scores[want] {
				t.Fatalf("%+v then %+v: score %s, want %s", first, second, score, scores[want])
			}
		}
	}
}

func TestShadowScoreOfMixedFlagsReportsTheFlaggedIntent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		intents []decisions.Intent
		reason  string
	}{
		{"flagged lower risk beats automatic higher risk", []decisions.Intent{{RiskClass: "R0", RequiresApproval: true}, {RiskClass: "R1"}}, "would_require_approval_r0"},
		{"flagged second of equal risk", []decisions.Intent{{RiskClass: "R1"}, {RiskClass: "R1", RequiresApproval: true}}, "would_require_approval_r1"},
		{"higher risk wins inside one route", []decisions.Intent{{RiskClass: "R0", RequiresApproval: true}, {RiskClass: "R1", RequiresApproval: true}}, "would_require_approval_r1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			score, reason := ScoreShadowDecision(&decisions.Result{Intents: test.intents})
			if score != ShadowWouldRequireApproval || reason != test.reason {
				t.Fatalf("score=%s reason=%s", score, reason)
			}
		})
	}
}
