package domain

import (
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// ShadowScore is the would-be policy result for a shadow decision: what
// EvaluateIntent WOULD have decided, computed but never written to intents or
// commands.
type ShadowScore string

// Shadow scores.
const (
	ShadowWouldApprove         ShadowScore = "would_approve"
	ShadowWouldRequireApproval ShadowScore = "would_require_approval"
	ShadowWouldDeny            ShadowScore = "would_deny"
)

// ShadowDecision is one scored shadow dispatch. A shadow dispatch is scored and
// recorded and NEVER written to intents or commands: nothing from a shadow run
// enters action governance.
type ShadowDecision struct {
	ShadowDecisionID string
	EpisodeID        string
	DecisionID       string
	AttemptID        string
	Fence            int64
	DecisionJSON     []byte
	DecisionSHA256   []byte
	ShadowScore      ShadowScore
	ScoreReason      string
	TenantID         string
	SituationID      string
	SituationVersion int
	PolicyEpoch      string
}

// ScoreShadowDecision is the would-be policy outcome of a shadow decision: the
// highest-risk intent's route under the live policy, including its catalog
// requires_approval flag.
func ScoreShadowDecision(validated *decisions.Result) (ShadowScore, string) {
	highest := highestRiskIntent(validated)
	risk := contractsv1.RiskClass(highest.RiskClass)
	switch contractsv1.RouteFor(risk, highest.RequiresApproval) {
	case contractsv1.RouteAutomatic:
		return ShadowWouldApprove, "would_approve_" + highest.RiskClass
	case contractsv1.RouteApproval:
		return ShadowWouldRequireApproval, "would_require_approval_" + strings.ToLower(highest.RiskClass)
	default:
		return ShadowWouldDeny, "would_deny_" + highest.RiskClass
	}
}

// highestRiskIntent returns the validated decision's highest-risk intent.
func highestRiskIntent(validated *decisions.Result) decisions.Intent {
	highest := validated.Intents[0]
	for _, intent := range validated.Intents[1:] {
		if contractsv1.RiskClass(intent.RiskClass).Rank() > contractsv1.RiskClass(highest.RiskClass).Rank() {
			highest = intent
		}
	}
	return highest
}

// ShadowDecisionIdentity binds a shadow row to the decision it scored.
type ShadowDecisionIdentity struct {
	ShadowDecisionID string
	EpisodeID        string
	DecisionID       string
	TenantID         string
	SituationID      string
	SituationVersion int
	PolicyEpoch      string
}

// NewShadowDecision builds the report-only shadow row for one scored
// decision; nothing from it enters action governance.
func NewShadowDecision(identity ShadowDecisionIdentity, attempt episodeledger.Identity, decisionJSON []byte, decisionSHA []byte, score ShadowScore, reason string) ShadowDecision {
	return ShadowDecision{
		ShadowDecisionID: identity.ShadowDecisionID,
		EpisodeID:        identity.EpisodeID, DecisionID: identity.DecisionID, AttemptID: attempt.AttemptID, Fence: attempt.Fence,
		DecisionJSON: decisionJSON, DecisionSHA256: decisionSHA,
		ShadowScore: score, ScoreReason: reason,
		TenantID: identity.TenantID, SituationID: identity.SituationID, SituationVersion: identity.SituationVersion,
		PolicyEpoch: identity.PolicyEpoch,
	}
}
