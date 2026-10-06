package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
)

// Ceiling is a stored cost limit row's ceiling and kill switch.
type Ceiling struct {
	Scope      string
	MaxMicro   int64
	KillSwitch bool
}

// InsertReservation records the reserved cost of an episode.
func (t *Tx) InsertReservation(ctx context.Context, episodeID, tenantID string, reserved int64, now string) error {
	if _, err := t.q.ExecContext(ctx, `
		INSERT INTO cost_reservations (reservation_id, episode_id, tenant_id, reserved_micro, status, created_at)
		VALUES (?, ?, ?, ?, 'reserved', ?)`, episodeID, episodeID, tenantID, reserved, now); err != nil {
		return fmt.Errorf("insert cost reservation: %w", err)
	}
	return nil
}

// LoadReservation reads an episode's reservation; found is false when it has none.
func (t *Tx) LoadReservation(ctx context.Context, episodeID string) (domain.Reservation, bool, error) {
	var r domain.Reservation
	err := t.q.QueryRowContext(ctx, "SELECT tenant_id, reserved_micro, actual_micro, status FROM cost_reservations WHERE episode_id = ?", episodeID).Scan(&r.TenantID, &r.Reserved, &r.Actual, &r.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reservation{}, false, nil
	}
	if err != nil {
		return domain.Reservation{}, false, fmt.Errorf("load cost reservation: %w", err)
	}
	return r, true, nil
}

// RecordSettlement marks a reserved reservation settled with its actual cost.
func (t *Tx) RecordSettlement(ctx context.Context, episodeID string, actual int64, now string) error {
	if _, err := t.q.ExecContext(ctx, `
		UPDATE cost_reservations SET actual_micro = ?, status = 'settled', settled_at = ?
		WHERE episode_id = ? AND status = 'reserved'`, actual, now, episodeID); err != nil {
		return fmt.Errorf("settle cost reservation: %w", err)
	}
	return nil
}

// CeilingsOf reads the ceilings of the given scopes, closing the result set
// before returning.
func (t *Tx) CeilingsOf(ctx context.Context, globalScope, tenantScope string) ([]Ceiling, error) {
	rows, err := t.q.QueryContext(ctx, "SELECT scope_key, max_micro, kill_switch FROM cost_limits WHERE scope_key IN (?, ?)", globalScope, tenantScope)
	if err != nil {
		return nil, fmt.Errorf("read aggregate cost ceilings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ceilings, err := collectCeilings(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read aggregate cost ceilings: %w", err)
	}
	return ceilings, nil
}

func collectCeilings(rows *sql.Rows) ([]Ceiling, error) {
	var ceilings []Ceiling
	for rows.Next() {
		var ceiling Ceiling
		var kill int64
		if err := rows.Scan(&ceiling.Scope, &ceiling.MaxMicro, &kill); err != nil {
			return nil, fmt.Errorf("scan %s cost ceiling: %w", ceiling.Scope, err)
		}
		ceiling.KillSwitch = kill != 0
		ceilings = append(ceilings, ceiling)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read aggregate cost ceilings: %w", err)
	}
	return ceilings, nil
}
