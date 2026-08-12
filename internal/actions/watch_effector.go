package actions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// WatchEffector installs bounded, expiring derived triggers. It has no
// reference to a spec store or deployment path by construction.
type WatchEffector struct {
	db  *storage.DB
	clk clock.Clock
}

// NewWatchEffector creates an internal watch-condition effector.
func NewWatchEffector(db *storage.DB) *WatchEffector {
	return NewWatchEffectorWithClock(db, clock.Physical())
}

// NewWatchEffectorWithClock creates a watch effector with deterministic time.
func NewWatchEffectorWithClock(db *storage.DB, clk clock.Clock) *WatchEffector {
	if clk == nil {
		clk = clock.Physical()
	}
	return &WatchEffector{db: db, clk: clk}
}

// Dispatch installs one watch condition and is idempotent by watch_id.
func (e *WatchEffector) Dispatch(ctx context.Context, command Command) (Effect, error) {
	return e.dispatch(ctx, command, nil)
}

// DispatchAuthorized performs the final authorization check before install.
func (e *WatchEffector) DispatchAuthorized(ctx context.Context, command Command, authorization Authorization) (Effect, error) {
	if authorization.Check == nil {
		return Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	if err := authorization.Check(ctx); err != nil {
		return Effect{}, err
	}
	return e.dispatch(ctx, command, authorization.Check)
}

func (e *WatchEffector) dispatch(ctx context.Context, command Command, _ func(context.Context) error) (Effect, error) {
	if e == nil || e.db == nil {
		return Effect{}, fmt.Errorf("watch effector storage is required")
	}
	if command.EffectorRoute != "install_watch_condition" {
		return Effect{}, fmt.Errorf("watch effector does not support route %q", command.EffectorRoute)
	}
	expression, _ := command.Payload["expression"].(string)
	target, _ := command.Payload["target"].(string)
	expiresAt, _ := command.Payload["expires_at"].(string)
	situationID, _ := command.Payload["situation_id"].(string)
	situationVersion, ok := integerPayload(command.Payload["situation_version"])
	remaining, remainingOK := integerPayload(command.Payload["max_fires"])
	if expression == "" || len(expression) > 4096 || target == "" || situationID == "" || !ok || situationVersion < 1 || !remainingOK || remaining < 1 || remaining > 100 {
		return Effect{}, fmt.Errorf("watch condition payload is invalid")
	}
	parsedExpiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !parsedExpiry.After(e.clk.Now().UTC()) {
		return Effect{}, fmt.Errorf("watch condition expiry is invalid")
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO watch_conditions (
				watch_id, tenant_id, situation_id, situation_version, expression, target,
				expires_at, remaining_fires, status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
			ON CONFLICT(watch_id) DO NOTHING`,
			command.CommandID, command.TenantID, situationID, situationVersion, expression, target,
			expiresAt, remaining, now, now)
		if err != nil {
			return fmt.Errorf("install watch condition: %w", err)
		}
		return nil
	}); err != nil {
		return Effect{}, fmt.Errorf("watch condition transaction: %w", err)
	}
	return Effect{ProviderResult: map[string]any{"accepted": true, "watch_id": command.CommandID}}, nil
}

// Fire records one event-driven watch firing exactly once and decrements its
// bounded allowance. It returns false for expired, disabled, or duplicate fires.
func (e *WatchEffector) Fire(ctx context.Context, watchID, eventID, now string) (bool, error) {
	if e == nil || e.db == nil || watchID == "" || eventID == "" || now == "" {
		return false, fmt.Errorf("watch identity and time are required")
	}
	fired := false
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO watch_fires (watch_id, event_id, fired_at) SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM watch_conditions WHERE watch_id = ? AND status = 'active' AND expires_at > ? AND remaining_fires > 0) ON CONFLICT(watch_id, event_id) DO NOTHING`, watchID, eventID, now, watchID, now)
		if err != nil {
			return fmt.Errorf("record watch fire: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count watch fire: %w", err)
		}
		if count != 1 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE watch_conditions SET remaining_fires = remaining_fires - 1, status = CASE WHEN remaining_fires = 1 THEN 'disabled' ELSE status END, updated_at = ? WHERE watch_id = ?`, now, watchID); err != nil {
			return fmt.Errorf("decrement watch allowance: %w", err)
		}
		fired = true
		return nil
	}); err != nil {
		return false, fmt.Errorf("watch fire transaction: %w", err)
	}
	return fired, nil
}

// Expire marks due active watches inactive without deleting their audit rows.
func (e *WatchEffector) Expire(ctx context.Context, now string) error {
	if e == nil || e.db == nil || now == "" {
		return fmt.Errorf("watch storage and time are required")
	}
	if _, err := e.db.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", now, now); err != nil {
		return fmt.Errorf("expire watch conditions: %w", err)
	}
	return nil
}

func integerPayload(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), number == float64(int(number))
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
