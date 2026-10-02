package episodes

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

func (r *Runner) persistValidatedIntents(ctx context.Context, tx *sql.Tx, validated *decisions.Result, req *Request, now string) error {
	for _, intent := range validated.Intents {
		digest, err := canonicaljson.DecodeDigest(intent.Digest)
		if err != nil {
			return fmt.Errorf("decode intent digest: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO intents (
				intent_id, decision_id, tenant_id, situation_id, situation_version,
				intent_type, risk_class, intent_json, intent_sha256, expires_at,
				rate_limit_per_hour, requires_approval, policy_status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
			intent.ID, validated.DecisionID, req.TenantID, req.SituationID, req.SituationVersion,
			intent.Type, intent.RiskClass, intent.CanonicalJSON, digest,
			intent.ExpiresAt.UTC().Format(time.RFC3339Nano), intent.RateLimitPerHour,
			boolToInt(intent.RequiresApproval), now, now,
		); err != nil {
			return fmt.Errorf("insert intent %s: %w", intent.ID, err)
		}
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// recordShadow scores a shadow decision: the would-be policy outcome computed
// from the validated intents, persisted ONLY to the shadow_decisions table —
// never to intents or commands. The score is the highest-risk intent's would-be
// result under the live policy (R0/R1 automatic, R2 requires approval, R3/R4
// denied). The decision is correlated by decision_id, so a shadow row is
// traceable to the decision it scored.
func (r *Runner) recordShadow(ctx context.Context, tx *sql.Tx, decisionID string, decisionDigest []byte, req *Request, outcome *Outcome, validated *decisions.Result, now string) error {
	// decisions.Validate rejects a decision with zero intents, so the first
	// intent is always present here.
	highest := validated.Intents[0]
	for _, intent := range validated.Intents[1:] {
		if intentRiskRanks[intent.RiskClass] > intentRiskRanks[highest.RiskClass] {
			highest = intent
		}
	}
	var score storage.ShadowScore
	var reason string
	switch highest.RiskClass {
	case "R0", "R1":
		score = storage.ShadowWouldApprove
		reason = "would_approve_" + highest.RiskClass
	case "R2":
		score = storage.ShadowWouldRequireApproval
		reason = "would_require_approval_r2"
	default:
		score = storage.ShadowWouldDeny
		reason = "would_deny_" + highest.RiskClass
	}
	decisionSHA := decisionDigest
	if r.shadowStore == nil {
		return fmt.Errorf("shadow dispatch but no shadow store configured — scores would be silently dropped")
	}
	shadow := storage.ShadowDecision{
		ShadowDecisionID: r.idGen.New(ids.PrefixShadow),
		EpisodeID:        req.EpisodeID,
		DecisionID:       decisionID,
		AttemptID:        req.AttemptID,
		Fence:            req.Fence,
		DecisionJSON:     outcome.DecisionJSON,
		DecisionSHA256:   decisionSHA,
		ShadowScore:      score,
		ScoreReason:      reason,
		TenantID:         req.TenantID,
		SituationID:      req.SituationID,
		SituationVersion: req.SituationVersion,
		PolicyEpoch:      req.PolicyEpoch,
	}
	if err := r.shadowStore.Record(ctx, tx, shadow, now); err != nil {
		return fmt.Errorf("record shadow decision: %w", err)
	}
	return nil
}
