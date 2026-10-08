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
	return appendEvaluationReason(ctx, tx, itemID, func(reasons []string) []string {
		return domain.RecordCostRefusal(reasons, rejection.Error())
	})
}

func RecordSchedulerExpiryReason(ctx context.Context, tx *store.Tx, itemID, reason string) error {
	if !tx.Configured() || reason == "" {
		return fmt.Errorf("cognition scheduler expiry requires transaction and reason")
	}
	return appendEvaluationReason(ctx, tx, itemID, func(reasons []string) []string {
		return domain.RecordSchedulerExpiry(reasons, reason)
	})
}

func appendEvaluationReason(ctx context.Context, tx *store.Tx, itemID string, add func([]string) []string) error {
	trigger, reasons, err := tx.LoadEvaluationReasons(ctx, itemID)
	if err != nil {
		return err
	}
	return tx.RecordReasons(ctx, trigger, add(reasons))
}
