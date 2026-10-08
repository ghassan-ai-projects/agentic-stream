package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// RecordShadowDecision persists one scored shadow decision and its would-be
// score inside the caller's transaction, correlated to the decision it scored.
// It writes only shadow_decisions: never intents, commands or the outbox.
func (tx *Tx) RecordShadowDecision(ctx context.Context, decision domain.ShadowDecision, now time.Time) error {
	if _, err := tx.tx.ExecContext(ctx, `
		INSERT INTO shadow_decisions (
			shadow_decision_id, episode_id, decision_id, attempt_id, fence, decision_json,
			decision_sha256, shadow_score, score_reason, tenant_id, situation_id,
			situation_version, policy_epoch, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		decision.ShadowDecisionID, decision.EpisodeID, decision.DecisionID, decision.AttemptID, decision.Fence,
		decision.DecisionJSON, decision.DecisionSHA256, string(decision.ShadowScore),
		decision.ScoreReason, decision.TenantID, decision.SituationID,
		decision.SituationVersion, decision.PolicyEpoch, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("record shadow decision: %w", err)
	}
	return nil
}
