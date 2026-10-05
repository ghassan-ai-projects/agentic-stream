package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

// RiskRank orders intent risk classes for shadow scoring and catalog
// admission.
var RiskRank = map[string]int{"R0": 0, "R1": 1, "R2": 2, "R3": 3, "R4": 4}

// ShadowScore is the would-be policy outcome of a shadow decision: the
// highest-risk intent's result under the live policy.
func ShadowScore(validated *decisions.Result) (qualification.ShadowScore, string) {
	highest := HighestRiskIntent(validated)
	switch highest.RiskClass {
	case "R0", "R1":
		return qualification.ShadowWouldApprove, "would_approve_" + highest.RiskClass
	case "R2":
		return qualification.ShadowWouldRequireApproval, "would_require_approval_r2"
	default:
		return qualification.ShadowWouldDeny, "would_deny_" + highest.RiskClass
	}
}

// HighestRiskIntent returns the validated decision's highest-risk intent.
func HighestRiskIntent(validated *decisions.Result) decisions.Intent {
	highest := validated.Intents[0]
	for _, intent := range validated.Intents[1:] {
		if RiskRank[intent.RiskClass] > RiskRank[highest.RiskClass] {
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
func NewShadowDecision(identity ShadowDecisionIdentity, attempt episodeledger.Identity, decisionJSON []byte, decisionSHA []byte, score qualification.ShadowScore, reason string) qualification.ShadowDecision {
	return qualification.ShadowDecision{
		ShadowDecisionID: identity.ShadowDecisionID,
		EpisodeID:        identity.EpisodeID, DecisionID: identity.DecisionID, AttemptID: attempt.AttemptID, Fence: attempt.Fence,
		DecisionJSON: decisionJSON, DecisionSHA256: decisionSHA,
		ShadowScore: score, ScoreReason: reason,
		TenantID: identity.TenantID, SituationID: identity.SituationID, SituationVersion: identity.SituationVersion,
		PolicyEpoch: identity.PolicyEpoch,
	}
}
