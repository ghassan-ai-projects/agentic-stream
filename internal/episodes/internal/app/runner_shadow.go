package app

import (
	"context"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

func (r *Runner) persistValidatedIntents(ctx context.Context, tx *store.Tx, validated *decisions.Result, req *Request, now string) error {
	for _, intent := range validated.Intents {
		row := store.ValidatedIntentInsert{Intent: intent, DecisionID: validated.DecisionID, TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: req.SituationVersion, Now: now}
		if err := store.InsertValidatedIntent(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

// recordShadow scores a shadow decision and persists it ONLY to the
// shadow_decisions table — never to intents or commands. The score is the
// highest-risk intent's would-be result under the live policy.
func (r *Runner) recordShadow(ctx context.Context, tx *store.Tx, decisionID string, decisionDigest []byte, req *Request, outcome *Outcome, validated *decisions.Result, now string) error {
	// decisions.Validate rejects a decision with zero intents, so the first
	// intent is always present here.
	score, reason := domain.ScoreShadowDecision(validated)
	shadow := domain.NewShadowDecision(r.shadowIdentity(decisionID, req), reqIdentity(req), outcome.DecisionJSON, decisionDigest, score, reason)
	return tx.RecordShadowDecision(ctx, shadow, now)
}

// reqIdentity projects the request's bound worker identity.
func reqIdentity(req *Request) episodeledger.Identity {
	return episodeledger.Identity{EpisodeID: req.EpisodeID, AttemptID: req.AttemptID, Fence: req.Fence}
}

func (r *Runner) shadowIdentity(decisionID string, req *Request) domain.ShadowDecisionIdentity {
	return domain.ShadowDecisionIdentity{
		ShadowDecisionID: r.idGen.New(ids.PrefixShadow), EpisodeID: req.EpisodeID, DecisionID: decisionID,
		TenantID: req.TenantID, SituationID: req.SituationID,
		SituationVersion: req.SituationVersion, PolicyEpoch: req.PolicyEpoch,
	}
}
