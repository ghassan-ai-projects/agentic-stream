package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// ShadowScore is the would-be policy result for a shadow decision: what
// EvaluateIntent WOULD have decided, computed but never written to intents or
// commands.
type ShadowScore string

const (
	ShadowWouldApprove         ShadowScore = "would_approve"
	ShadowWouldRequireApproval ShadowScore = "would_require_approval"
	ShadowWouldDeny            ShadowScore = "would_deny"
)

// ShadowDecision is one scored shadow dispatch.
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

// ShadowStore persists scored shadow decisions. A shadow dispatch is scored
// and recorded here and NEVER written to intents or commands — nothing from a
// shadow run enters action governance.
type ShadowStore struct {
	DB *DB
}

// Record persists a shadow decision and its would-be score, correlated to the
// decision it scored.
func (s *ShadowStore) Record(ctx context.Context, tx *sql.Tx, decision ShadowDecision, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO shadow_decisions (
			shadow_decision_id, episode_id, decision_id, attempt_id, fence, decision_json,
			decision_sha256, shadow_score, score_reason, tenant_id, situation_id,
			situation_version, policy_epoch, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		decision.ShadowDecisionID, decision.EpisodeID, decision.DecisionID, decision.AttemptID, decision.Fence,
		decision.DecisionJSON, decision.DecisionSHA256, string(decision.ShadowScore),
		decision.ScoreReason, decision.TenantID, decision.SituationID,
		decision.SituationVersion, decision.PolicyEpoch, now); err != nil {
		return fmt.Errorf("record shadow decision: %w", err)
	}
	return nil
}

// CountShadowDecisions returns the number of scored shadow decisions for an
// episode (test/audit surface).
func (s *ShadowStore) CountShadowDecisions(ctx context.Context, episodeID string) (int, error) {
	if s == nil || s.DB == nil {
		return 0, fmt.Errorf("shadow store is not configured")
	}
	var count int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM shadow_decisions WHERE episode_id = ?`, episodeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count shadow decisions: %w", err)
	}
	return count, nil
}
