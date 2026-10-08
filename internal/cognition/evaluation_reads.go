package cognition

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

// TriggerEvaluationRecord is one durable trigger evaluation: what the trigger
// saw (the delta), its score against the threshold, and why it decided.
type TriggerEvaluationRecord = domain.TriggerEvaluationRecord

// TriggerEvaluation reads one of the tenant's trigger evaluations.
func TriggerEvaluation(ctx context.Context, db *sql.DB, tenantID, triggerID string) (TriggerEvaluationRecord, error) {
	return app.TriggerEvaluation(ctx, store.NewReader(db), tenantID, triggerID)
}

// TriggerEvaluations reads every trigger evaluation of one Situation version.
func TriggerEvaluations(ctx context.Context, db *sql.DB, tenantID, situationID string, version int) ([]TriggerEvaluationRecord, error) {
	return app.TriggerEvaluations(ctx, store.NewReader(db), tenantID, situationID, version)
}
