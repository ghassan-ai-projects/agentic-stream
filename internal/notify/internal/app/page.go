package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

// ReadPage returns up to Limit events strictly after the cursor. It refuses a
// resume cursor that predates retained data, and when MaxLag is positive it
// disconnects a subscriber lagging further behind; both refusals are audited.
// A record that fails its digest, decoding, or validation is a poison record:
// it is skipped once its retry budget is spent and otherwise fails the page.
func (s *Service) ReadPage(ctx context.Context, request domain.PageRequest, now time.Time) (domain.Page, error) {
	if err := s.checkRequest(ctx, request, now); err != nil {
		return domain.Page{}, err
	}
	rows, err := s.store.Autocommit().ReadRows(ctx, request.TenantID, request.Cursor, request.Limit)
	if err != nil {
		return domain.Page{}, err
	}
	page := domain.Page{Records: make([]domain.Record, 0, request.Limit), NextCursor: request.Cursor}
	for _, row := range rows {
		if err := s.admit(ctx, &page, request.TenantID, row, now); err != nil {
			return domain.Page{}, err
		}
	}
	return page, nil
}

// checkRequest refuses a bad limit, an expired cursor or, when MaxLag is
// positive, a subscriber that lags too far behind, auditing each refusal.
func (s *Service) checkRequest(ctx context.Context, request domain.PageRequest, now time.Time) error {
	if err := domain.CheckPageLimit(request.Limit); err != nil {
		return err
	}
	bounds, err := s.store.Autocommit().TenantBounds(ctx, request.TenantID)
	if err != nil {
		return err
	}
	refusal, refused := domain.RefuseResume(request.Cursor, request.MaxLag, retained(bounds))
	if !refused {
		return nil
	}
	if err := s.auditRefusal(ctx, request, bounds, refusal, now); err != nil {
		return fmt.Errorf("audit %s: %w", refusal.Subject, err)
	}
	return refusal.Err
}

func retained(bounds store.Bounds) domain.Retained {
	return domain.Retained{Oldest: bounds.Oldest, HasOldest: bounds.HasOldest, NextCursor: bounds.NextCursor, HasNextCursor: bounds.HasNextCursor}
}

func (s *Service) admit(ctx context.Context, page *domain.Page, tenantID string, row store.Row, now time.Time) error {
	record, valid := domain.DecodeRecord(tenantID, row.Cursor, row.EventJSON, row.EventSHA)
	if !valid {
		return s.admitPoison(ctx, page, tenantID, row.Cursor, now)
	}
	if err := s.store.Autocommit().ClearPoisonAttempts(ctx, tenantID, row.Cursor); err != nil {
		return err
	}
	page.Deliver(record)
	return nil
}
