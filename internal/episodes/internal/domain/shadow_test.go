package domain

import (
	"testing"

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
