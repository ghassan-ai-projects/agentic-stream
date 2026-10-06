package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

// admitPoison counts a delivery attempt of a malformed record: it fails the
// page until the retry budget is spent, then skips the record.
func (s *Service) admitPoison(ctx context.Context, page *domain.Page, tenantID string, cursor int64, now time.Time) error {
	skip, err := s.recordPoisonAttempt(ctx, tenantID, cursor, now)
	if err != nil {
		return err
	}
	if !skip {
		return domain.ErrNotificationPoison
	}
	page.Skip(cursor)
	return nil
}

func (s *Service) recordPoisonAttempt(ctx context.Context, tenantID string, cursor int64, now time.Time) (bool, error) {
	var skip bool
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		skip, err = s.countPoisonAttempt(ctx, tx, tenantID, cursor, now)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("persist notification poison attempt: %w", err)
	}
	return skip, nil
}

// countPoisonAttempt records the attempt and, once the budget is spent, audits
// the skip and clears the counter in the same transaction.
func (s *Service) countPoisonAttempt(ctx context.Context, tx *store.Tx, tenantID string, cursor int64, now time.Time) (bool, error) {
	attempts, err := tx.CountPoisonAttempt(ctx, tenantID, cursor, now)
	if err != nil {
		return false, err
	}
	if !domain.PoisonSpent(attempts) {
		return false, nil
	}
	if err := s.writeAudit(ctx, tx, tenantID, domain.AuditSubscriberSkipped, cursor, cursor, now); err != nil {
		return false, fmt.Errorf("audit skipped poison notification: %w", err)
	}
	return true, tx.ClearPoisonAttempts(ctx, tenantID, cursor)
}
