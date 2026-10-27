package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
)

func (e *Service) QueueAudit(ctx context.Context, tx *store.Tx, request domain.AuditRequest) (string, error) {
	if !tx.Configured() {
		return "", fmt.Errorf("cognition caller transaction is required")
	}
	now := e.clk.Now().UTC()
	eval := domain.AuditEvaluation(request, e.spec.Digest, now)
	if err := e.scheduler.saveEvaluation(ctx, tx, eval, request.TenantID, e.deploymentID); err != nil {
		return "", fmt.Errorf("save audit evaluation: %w", err)
	}
	item := domain.AuditItem(eval, now)
	if err := e.scheduler.insertItem(ctx, tx, item, request.TenantID); err != nil {
		return "", fmt.Errorf("queue audit item: %w", err)
	}
	return item.SchedulerItemID, nil
}
