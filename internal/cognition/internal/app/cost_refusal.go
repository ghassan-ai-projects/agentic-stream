package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

func RecordCostRejectionReason(ctx context.Context, tx *store.Tx, itemID string, rejection error) error {
	if !tx.Configured() || rejection == nil {
		return fmt.Errorf("cognition cost refusal requires transaction and reason")
	}
	trigger, reasons, err := tx.LoadEvaluationReasons(ctx, itemID)
	if err != nil {
		return err
	}
	return tx.RecordCostReason(ctx, trigger, domain.RecordCostRefusal(reasons, rejection.Error()))
}
