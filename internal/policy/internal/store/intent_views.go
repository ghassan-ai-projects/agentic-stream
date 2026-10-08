package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

// Reader reads intents and their evaluations without a transaction.
type Reader struct{ db *sql.DB }

// NewReader binds an intent reader to an open database.
func NewReader(db *sql.DB) Reader { return Reader{db: db} }

const intentViewSQL = `
	SELECT intent_id, decision_id, situation_id, situation_version, intent_type, risk_class, policy_status,
		requires_approval, expires_at, intent_json, created_at
	FROM intents WHERE tenant_id = ?`

// Intent reads one of the tenant's intents with its evaluations.
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

// DecisionIntents reads the intents one Decision proposed, with their
// evaluations.
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
	views := []domain.IntentView{}
	for rows.Next() {
		var v domain.IntentView
		if err := rows.Scan(&v.IntentID, &v.DecisionID, &v.SituationID, &v.SituationVersion, &v.IntentType, &v.RiskClass, &v.PolicyStatus, &v.RequiresApproval, &v.ExpiresAt, &v.Intent, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan intent: %w", err)
		}
		views = append(views, v)
	}
	return views, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}

func (r Reader) evaluations(ctx context.Context, intentID string) ([]domain.PolicyEvaluationView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT evaluation_id, result, reason, policy_version, policy_digest, COALESCE(command_id, ''), COALESCE(approval_id, ''), situation_version, evaluated_at
		FROM policy_evaluations WHERE intent_id = ? ORDER BY evaluated_at, evaluation_id`, intentID)
	if err != nil {
		return nil, fmt.Errorf("read policy evaluations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	evaluations := []domain.PolicyEvaluationView{}
	for rows.Next() {
		var e domain.PolicyEvaluationView
		if err := rows.Scan(&e.EvaluationID, &e.Result, &e.Reason, &e.PolicyVersion, &e.PolicyDigest, &e.CommandID, &e.ApprovalID, &e.SituationVersion, &e.EvaluatedAt); err != nil {
			return nil, fmt.Errorf("scan policy evaluation: %w", err)
		}
		evaluations = append(evaluations, e)
	}
	return evaluations, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}
