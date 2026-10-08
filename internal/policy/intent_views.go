package policy

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// IntentView is one intent with its policy status and every policy
// evaluation and reason.
type IntentView = domain.IntentView

// PolicyEvaluationView is one policy evaluation of an intent and its reason.
type PolicyEvaluationView = domain.PolicyEvaluationView

// Intent reads one of the tenant's intents with its evaluations. It only
// reads.
func Intent(ctx context.Context, db *sql.DB, tenantID, intentID string) (IntentView, error) {
	return app.Intent(ctx, store.NewReader(db), tenantID, intentID)
}

// DecisionIntents reads the intents one Decision proposed, with their
// evaluations. It only reads.
func DecisionIntents(ctx context.Context, db *sql.DB, tenantID, decisionID string) ([]IntentView, error) {
	return app.DecisionIntents(ctx, store.NewReader(db), tenantID, decisionID)
}
