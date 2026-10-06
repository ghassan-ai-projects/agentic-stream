package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

// auditRefusal records a refused resume cursor in its own transaction.
func (s *Service) auditRefusal(ctx context.Context, request domain.PageRequest, bounds store.Bounds, refusal domain.Refusal, now time.Time) error {
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.writeAudit(ctx, tx, request.TenantID, refusal.Action, request.Cursor, bounds.Oldest, now)
	})
	if err != nil {
		return fmt.Errorf("write notification audit: %w", err)
	}
	return nil
}

func (s *Service) writeAudit(ctx context.Context, tx *store.Tx, tenantID, action string, requested, oldest int64, now time.Time) error {
	details, err := domain.AuditDetails(requested, oldest)
	if err != nil {
		return err
	}
	return tx.RecordAudit(ctx, store.Audit{ID: s.newAuditID(), TenantID: tenantID, Action: action, Requested: requested, Oldest: oldest, Details: details, At: now})
}
