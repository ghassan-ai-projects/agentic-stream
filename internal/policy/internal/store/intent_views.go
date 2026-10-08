package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

type Reader struct{ db *sql.DB }

func NewReader(db *sql.DB) Reader { return Reader{db: db} }

const intentViewSQL = `
	SELECT intent_id, decision_id, situation_id, situation_version, intent_type, risk_class, policy_status,
		requires_approval, expires_at, intent_json, created_at
	FROM intents WHERE tenant_id = ?`

func (r Reader) Intent(ctx context.Context, tenantID, intentID string) (domain.IntentView, error) {
	intents, err := r.intents(ctx, intentViewSQL+` AND intent_id = ?`, tenantID, intentID)
	if err != nil {
		return domain.IntentView{}, err
	}
	if len(intents) == 0 {
		return domain.IntentView{}, fmt.Errorf("intent %s: %w", intentID, sql.ErrNoRows)
	}
	return intents[0], nil
}

func (r Reader) DecisionIntents(ctx context.Context, tenantID, decisionID string) ([]domain.IntentView, error) {
	return r.intents(ctx, intentViewSQL+` AND decision_id = ? ORDER BY created_at, intent_id`, tenantID, decisionID)
}

func (r Reader) intents(ctx context.Context, query string, args ...any) ([]domain.IntentView, error) {
	views, err := r.scanIntents(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	for i := range views {
		if views[i].Evaluations, err = r.evaluations(ctx, views[i].IntentID); err != nil {
			return nil, err
		}
	}
	return views, nil
}

func (r Reader) scanIntents(ctx context.Context, query string, args ...any) ([]domain.IntentView, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read intents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	views, err := storage.CollectRows(rows, "intents", scanIntentView)
	if err != nil {
		return nil, fmt.Errorf("read intents: %w", err)
	}
	return views, nil
}

func scanIntentView(rows *sql.Rows) (domain.IntentView, error) {
	var v domain.IntentView
	if err := rows.Scan(&v.IntentID, &v.DecisionID, &v.SituationID, &v.SituationVersion, &v.IntentType, &v.RiskClass, &v.PolicyStatus, &v.RequiresApproval, &v.ExpiresAt, &v.Intent, &v.CreatedAt); err != nil {
		return domain.IntentView{}, fmt.Errorf("scan intent: %w", err)
	}
	return v, nil
}

func (r Reader) evaluations(ctx context.Context, intentID string) ([]domain.PolicyEvaluationView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT evaluation_id, result, reason, policy_version, policy_digest, COALESCE(command_id, ''), COALESCE(approval_id, ''), situation_version, evaluated_at
		FROM policy_evaluations WHERE intent_id = ? ORDER BY evaluated_at, evaluation_id`, intentID)
	if err != nil {
		return nil, fmt.Errorf("read policy evaluations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	evaluations, err := storage.CollectRows(rows, "policy evaluations", scanPolicyEvaluation)
	if err != nil {
		return nil, fmt.Errorf("read policy evaluations: %w", err)
	}
	return evaluations, nil
}

func scanPolicyEvaluation(rows *sql.Rows) (domain.PolicyEvaluationView, error) {
	var e domain.PolicyEvaluationView
	if err := rows.Scan(&e.EvaluationID, &e.Result, &e.Reason, &e.PolicyVersion, &e.PolicyDigest, &e.CommandID, &e.ApprovalID, &e.SituationVersion, &e.EvaluatedAt); err != nil {
		return domain.PolicyEvaluationView{}, fmt.Errorf("scan policy evaluation: %w", err)
	}
	return e, nil
}
