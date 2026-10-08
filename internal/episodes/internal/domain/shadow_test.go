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

func TestHighestRiskIntentFollowsTheRiskOrder(t *testing.T) {
	t.Parallel()
	classes := contractstest.RiskClasses()
	for index, want := range classes {
		validated := &decisions.Result{Intents: []decisions.Intent{{RiskClass: string(classes[0])}, {RiskClass: string(want)}, {RiskClass: string(classes[0])}}}
		if got := highestRiskIntent(validated).RiskClass; got != string(want) {
			t.Fatalf("index %d: highest = %s, want %s", index, got, want)
		}
	}
}
