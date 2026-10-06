package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

func TestShadowScoreUsesHighestRisk(t *testing.T) {
	t.Parallel()
	tests := []struct {
		risk, reason string
		score        qualification.ShadowScore
	}{
		{"R1", "would_approve_R1", qualification.ShadowWouldApprove},
		{"R2", "would_require_approval_r2", qualification.ShadowWouldRequireApproval},
		{"R4", "would_deny_R4", qualification.ShadowWouldDeny},
	}
	for _, test := range tests {
		t.Run(test.risk, func(t *testing.T) {
			t.Parallel()
			validated := &decisions.Result{Intents: []decisions.Intent{{RiskClass: "R0"}, {RiskClass: test.risk}, {RiskClass: "R0"}}}
			score, reason := ShadowScore(validated)
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
