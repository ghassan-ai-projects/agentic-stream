package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
)

// Reader is a Store that can only read: it has no owner fence or interlock.
func Reader(db *storage.DB) Store { return Store{db: db} }

// Watch reads one of the tenant's watches with its fires; found is false when
// no such watch was installed.
func (s Store) Watch(ctx context.Context, tenantID, watchID string) (domain.WatchView, bool, error) {
	var w domain.WatchView
	err := s.db.QueryRowContext(ctx, `SELECT watch_id, situation_id, situation_version, expression, target, status, expires_at, remaining_fires, max_fires
		FROM watch_conditions WHERE tenant_id = ? AND watch_id = ?`, tenantID, watchID).Scan(&w.WatchID, &w.SituationID, &w.SituationVersion, &w.Expression, &w.Target, &w.Status, &w.ExpiresAt, &w.RemainingFires, &w.MaxFires)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.WatchView{}, false, nil
	}
	if err != nil {
		return domain.WatchView{}, false, fmt.Errorf("read watch %s: %w", watchID, err)
	}
	w.Fires, err = s.fires(ctx, watchID)
	return w, err == nil, err
}

func (s Store) fires(ctx context.Context, watchID string) ([]domain.FireView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, fired_at FROM watch_fires WHERE watch_id = ? ORDER BY fired_at, event_id`, watchID)
	if err != nil {
		return nil, fmt.Errorf("read watch fires: %w", err)
	}
	defer func() { _ = rows.Close() }()
	fires := []domain.FireView{}
	for rows.Next() {
		var fire domain.FireView
		if err := rows.Scan(&fire.EventID, &fire.FiredAt); err != nil {
			return nil, fmt.Errorf("scan watch fire: %w", err)
		}
		fires = append(fires, fire)
	}
	return fires, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}
