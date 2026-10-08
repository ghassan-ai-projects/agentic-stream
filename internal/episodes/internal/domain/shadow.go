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
// route of its strictest intent under the live policy, including the catalog
// requires_approval flag. A denied route outranks approval, approval outranks
// automatic, and risk rank breaks ties within one route.
func ScoreShadowDecision(validated *decisions.Result) (ShadowScore, string) {
	strictest := strictestIntent(validated)
	switch intentRoute(strictest) {
	case contractsv1.RouteAutomatic:
		return ShadowWouldApprove, "would_approve_" + strictest.RiskClass
	case contractsv1.RouteApproval:
		return ShadowWouldRequireApproval, "would_require_approval_" + strings.ToLower(strictest.RiskClass)
	default:
		return ShadowWouldDeny, "would_deny_" + strictest.RiskClass
	}
}

func strictestIntent(validated *decisions.Result) decisions.Intent {
	strictest := validated.Intents[0]
	for _, intent := range validated.Intents[1:] {
		if stricterThan(intent, strictest) {
			strictest = intent
		}
	}
	return strictest
}

func stricterThan(candidate, incumbent decisions.Intent) bool {
	if candidateStrictness, incumbentStrictness := routeStrictness(intentRoute(candidate)), routeStrictness(intentRoute(incumbent)); candidateStrictness != incumbentStrictness {
		return candidateStrictness > incumbentStrictness
	}
	return contractsv1.RiskClass(candidate.RiskClass).Rank() > contractsv1.RiskClass(incumbent.RiskClass).Rank()
}

func intentRoute(intent decisions.Intent) contractsv1.Route {
	return contractsv1.RouteFor(contractsv1.RiskClass(intent.RiskClass), intent.RequiresApproval)
}

func routeStrictness(route contractsv1.Route) int {
	switch route {
	case contractsv1.RouteAutomatic:
		return 0
	case contractsv1.RouteApproval:
		return 1
	default:
		return 2
	}
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
