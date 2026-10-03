package watch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Effector installs bounded, expiring derived triggers. It has no
// reference to a spec store or deployment path by construction.
type Effector struct {
	db         *storage.DB
	clk        clock.Clock
	owner      *runtimecontrol.RuntimeOwner
	ownerEpoch string
	interlock  interlock.Reader
}

// watchCondition is the identity-defining content of an installed watch.
type watchCondition struct {
	tenantID, situationID, expression, target, expiresAt string
	situationVersion, maxFires                           int
}

// WithRuntimeOwner fences all durable watch mutations to the active runtime
// epoch. Standalone tests may leave the owner unset.
func (e *Effector) WithRuntimeOwner(owner *runtimecontrol.RuntimeOwner, epoch string) *Effector {
	e.owner = owner
	e.ownerEpoch = epoch
	return e
}

// WithInterlock adds the final action-plane readiness check to watch writes.
func (e *Effector) WithInterlock(reader interlock.Reader) *Effector {
	e.interlock = reader
	return e
}

// NewEffector creates an internal watch-condition effector.
func NewEffector(db *storage.DB) *Effector {
	return NewEffectorWithClock(db, clock.Physical())
}

// NewEffectorWithClock creates a watch effector with deterministic time.
func NewEffectorWithClock(db *storage.DB, clk clock.Clock) *Effector {
	if clk == nil {
		clk = clock.Physical()
	}
	return &Effector{db: db, clk: clk}
}

// Dispatch installs one watch condition and is idempotent by watch_id.
func (e *Effector) Dispatch(ctx context.Context, command actionport.Command) (actionport.Effect, error) {
	return e.dispatch(ctx, command, nil)
}

// DispatchAuthorized performs the final authorization check before install.
func (e *Effector) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	if authorization.Check == nil {
		return actionport.Effect{}, fmt.Errorf("dispatch authorization is required")
	}
	if err := authorization.Check(ctx); err != nil {
		return actionport.Effect{}, fmt.Errorf("%w", err)
	}
	return e.dispatch(ctx, command, authorization.Check)
}

func (e *Effector) dispatch(ctx context.Context, command actionport.Command, _ func(context.Context) error) (actionport.Effect, error) {
	if e == nil || e.db == nil {
		return actionport.Effect{}, fmt.Errorf("watch effector storage is required")
	}
	if command.EffectorRoute != "install_watch_condition" {
		return actionport.Effect{}, fmt.Errorf("watch effector does not support route %q", command.EffectorRoute)
	}
	want, err := watchConditionFromCommand(command, e.clk.Now().UTC())
	if err != nil {
		return actionport.Effect{}, err
	}
	return e.installCommand(ctx, command, want)
}

func (e *Effector) installCommand(ctx context.Context, command actionport.Command, want watchCondition) (actionport.Effect, error) {
	watchID := command.CommandID
	if command.IdempotencyKey != "" {
		watchID = command.IdempotencyKey
	}
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	if err := e.db.WithTx(ctx, func(tx *sql.Tx) error {
		return e.installOnce(ctx, tx, watchID, want, now)
	}); err != nil {
		return actionport.Effect{}, fmt.Errorf("watch condition transaction: %w", err)
	}
	return actionport.Effect{ProviderResult: map[string]any{"accepted": true, "watch_id": watchID}}, nil
}

// installOnce installs the watch, or accepts an identical earlier install of
// the same watch ID and rejects a conflicting one.
func (e *Effector) installOnce(ctx context.Context, tx *sql.Tx, watchID string, want watchCondition, now string) error {
	existing, err := loadWatchCondition(ctx, tx, watchID)
	if err != nil {
		return fmt.Errorf("load existing watch condition: %w", err)
	}
	if existing != nil {
		return want.sameAs(*existing)
	}
	if err := e.assertGuards(ctx, tx, want.tenantID, want.target); err != nil {
		return err
	}
	if err := insertWatchCondition(ctx, tx, watchID, want, now); err != nil {
		return err
	}
	return verifyInstalledWatch(ctx, tx, watchID, want)
}

func verifyInstalledWatch(ctx context.Context, tx *sql.Tx, watchID string, want watchCondition) error {
	stored, err := loadWatchCondition(ctx, tx, watchID)
	if err != nil {
		return fmt.Errorf("verify installed watch condition: %w", err)
	}
	if stored == nil {
		return fmt.Errorf("verify installed watch condition: %w", sql.ErrNoRows)
	}
	return want.sameAs(*stored)
}

func insertWatchCondition(ctx context.Context, tx *sql.Tx, watchID string, want watchCondition, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO watch_conditions (
			watch_id, tenant_id, situation_id, situation_version, expression, target,
			expires_at, remaining_fires, max_fires, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
		ON CONFLICT(watch_id) DO NOTHING`,
		watchID, want.tenantID, want.situationID, want.situationVersion, want.expression, want.target,
		want.expiresAt, want.maxFires, want.maxFires, now, now); err != nil {
		return fmt.Errorf("install watch condition: %w", err)
	}
	return nil
}

// watchConditionFromCommand validates a watch payload: a bounded, valid
// expression, a target and Situation, 1-100 fires, and a future expiry.
func watchConditionFromCommand(command actionport.Command, now time.Time) (watchCondition, error) {
	condition := watchCondition{tenantID: command.TenantID}
	condition.expression, _ = command.Payload["expression"].(string)
	condition.target, _ = command.Payload["target"].(string)
	condition.situationID, _ = command.Payload["situation_id"].(string)
	expiresAt, _ := command.Payload["expires_at"].(string)
	var versionOK, firesOK bool
	condition.situationVersion, versionOK = integerPayload(command.Payload["situation_version"])
	condition.maxFires, firesOK = integerPayload(command.Payload["max_fires"])
	if condition.expression == "" || len(condition.expression) > 4096 || condition.target == "" || condition.situationID == "" ||
		!versionOK || condition.situationVersion < 1 || !firesOK || condition.maxFires < 1 || condition.maxFires > 100 {
		return watchCondition{}, fmt.Errorf("watch condition payload is invalid")
	}
	return condition.withValidatedExpiry(expiresAt, now)
}

func (condition watchCondition) withValidatedExpiry(expiresAt string, now time.Time) (watchCondition, error) {
	if err := validateWatchExpression(condition.expression); err != nil {
		return watchCondition{}, err
	}
	parsedExpiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !parsedExpiry.After(now) {
		return watchCondition{}, fmt.Errorf("watch condition expiry is invalid")
	}
	condition.expiresAt = parsedExpiry.UTC().Format(time.RFC3339Nano)
	return condition, nil
}

// loadWatchCondition returns the stored watch, or nil when none exists.
func loadWatchCondition(ctx context.Context, tx *sql.Tx, watchID string) (*watchCondition, error) {
	var stored watchCondition
	var remainingFires int
	err := tx.QueryRowContext(ctx, `SELECT tenant_id, situation_id, situation_version, expression, target, expires_at, remaining_fires, max_fires FROM watch_conditions WHERE watch_id = ?`, watchID).Scan(&stored.tenantID, &stored.situationID, &stored.situationVersion, &stored.expression, &stored.target, &stored.expiresAt, &remainingFires, &stored.maxFires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load watch condition: %w", err)
	}
	return &stored, nil
}

// sameAs makes a repeated install idempotent: the same watch ID must carry
// the same condition.
func (c watchCondition) sameAs(stored watchCondition) error {
	if c != stored {
		return fmt.Errorf("watch command idempotency conflict")
	}
	return nil
}

// assertGuards re-checks runtime ownership and the governance interlock.
func (e *Effector) assertGuards(ctx context.Context, tx *sql.Tx, tenantID, target string) error {
	if e.owner != nil && e.ownerEpoch != "" {
		if err := e.owner.Assert(ctx, tx, e.ownerEpoch); err != nil {
			return fmt.Errorf("assert watch runtime owner: %w", err)
		}
	}
	if e.interlock != nil {
		if err := e.interlock.Assert(ctx, tx, tenantID, target, ""); err != nil {
			return fmt.Errorf("assert watch interlock: %w", err)
		}
	}
	return nil
}
