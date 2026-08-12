package actions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
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
	if err := validateWatchExpression(expression); err != nil {
		return Effect{}, err
	}
	parsedExpiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !parsedExpiry.After(e.clk.Now().UTC()) {
		return Effect{}, fmt.Errorf("watch condition expiry is invalid")
	}
	expiresAt = parsedExpiry.UTC().Format(time.RFC3339Nano)
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		var existing struct {
			TenantID, SituationID, Expression, Target, ExpiresAt string
			SituationVersion, RemainingFires                     int
		}
		existingErr := tx.QueryRowContext(ctx, `SELECT tenant_id, situation_id, situation_version, expression, target, expires_at, remaining_fires FROM watch_conditions WHERE watch_id = ?`, command.CommandID).Scan(&existing.TenantID, &existing.SituationID, &existing.SituationVersion, &existing.Expression, &existing.Target, &existing.ExpiresAt, &existing.RemainingFires)
		if existingErr == nil {
			if existing.TenantID != command.TenantID || existing.SituationID != situationID || existing.SituationVersion != situationVersion || existing.Expression != expression || existing.Target != target || existing.ExpiresAt != expiresAt || existing.RemainingFires != remaining {
				return fmt.Errorf("watch command idempotency conflict")
			}
			return nil
		}
		if existingErr != sql.ErrNoRows {
			return fmt.Errorf("load existing watch condition: %w", existingErr)
		}
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
		var stored struct {
			TenantID, SituationID, Expression, Target, ExpiresAt string
			SituationVersion, RemainingFires                     int
		}
		if err := tx.QueryRowContext(ctx, `SELECT tenant_id, situation_id, situation_version, expression, target, expires_at, remaining_fires FROM watch_conditions WHERE watch_id = ?`, command.CommandID).Scan(&stored.TenantID, &stored.SituationID, &stored.SituationVersion, &stored.Expression, &stored.Target, &stored.ExpiresAt, &stored.RemainingFires); err != nil {
			return fmt.Errorf("verify installed watch condition: %w", err)
		}
		if stored.TenantID != command.TenantID || stored.SituationID != situationID || stored.SituationVersion != situationVersion || stored.Expression != expression || stored.Target != target || stored.ExpiresAt != expiresAt || stored.RemainingFires != remaining {
			return fmt.Errorf("watch command idempotency conflict")
		}
		return nil
	}); err != nil {
		return Effect{}, fmt.Errorf("watch condition transaction: %w", err)
	}
	return Effect{ProviderResult: map[string]any{"accepted": true, "watch_id": command.CommandID}}, nil
}

// Fire records one event-driven watch firing exactly once and decrements its
// bounded allowance. It returns false for expired, disabled, or duplicate fires.
func (e *WatchEffector) Fire(ctx context.Context, watchID, eventID, situationID, target string, features map[string]any) (bool, error) {
	if e == nil || e.db == nil || watchID == "" || eventID == "" {
		return false, fmt.Errorf("watch identity is required")
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	fired := false
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", now, now); err != nil {
			return fmt.Errorf("expire due watch conditions: %w", err)
		}
		var expression, storedSituationID, storedTarget string
		if err := tx.QueryRowContext(ctx, "SELECT expression, situation_id, target FROM watch_conditions WHERE watch_id = ? AND status = 'active' AND expires_at > ?", watchID, now).Scan(&expression, &storedSituationID, &storedTarget); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return fmt.Errorf("load active watch condition: %w", err)
		}
		if storedSituationID != situationID || storedTarget != target {
			return nil
		}
		matches, err := evaluateWatchExpression(expression, features)
		if err != nil {
			return err
		}
		if !matches {
			return nil
		}
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
func (e *WatchEffector) Expire(ctx context.Context) error {
	if e == nil || e.db == nil {
		return fmt.Errorf("watch storage is required")
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	if _, err := e.db.ExecContext(ctx, "UPDATE watch_conditions SET status = 'expired', updated_at = ? WHERE status = 'active' AND expires_at <= ?", now, now); err != nil {
		return fmt.Errorf("expire watch conditions: %w", err)
	}
	return nil
}

func validateWatchExpression(expression string) error {
	if strings.ContainsAny(expression, "{};`") {
		return fmt.Errorf("watch expression contains forbidden syntax")
	}
	env, err := cel.NewEnv(
		cel.Variable("features", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("situation", cel.MapType(cel.StringType, cel.DynType)),
		ext.Bindings(),
	)
	if err != nil {
		return fmt.Errorf("create watch expression environment: %w", err)
	}
	_, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("watch expression is not valid CEL: %w", issues.Err())
	}
	return nil
}

func evaluateWatchExpression(expression string, features map[string]any) (bool, error) {
	env, err := cel.NewEnv(
		cel.Variable("features", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("situation", cel.MapType(cel.StringType, cel.DynType)),
		ext.Bindings(),
	)
	if err != nil {
		return false, fmt.Errorf("create watch expression environment: %w", err)
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("watch expression is not valid CEL: %w", issues.Err())
	}
	program, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("build watch expression program: %w", err)
	}
	value, _, err := program.Eval(map[string]any{"features": features, "situation": map[string]any{}})
	if err != nil {
		return false, fmt.Errorf("evaluate watch expression: %w", err)
	}
	matched, ok := value.Value().(bool)
	if !ok {
		return false, fmt.Errorf("watch expression must return bool")
	}
	return matched, nil
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
