package actions

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// AwaitingCommand is a command whose outcome is uncertain.
type AwaitingCommand = app.AwaitingCommand

// ReconcilerConfig binds the database and the runtime owner fence; the
// reconciler never dispatches, so it takes no effector.
type ReconcilerConfig struct {
	DB           *storage.DB
	RuntimeOwner func(context.Context, *sql.Tx, string) error
	Epoch        string
}

// Reconciler closes commands whose outcome is uncertain (outcome_unknown,
// reconciling or manual_review) from evidence an operator supplies. A provider
// timeout is never retried blindly; this is how such a command is closed.
type Reconciler struct{ app *app.Reconciler }

// NewReconciler validates the configuration.
func NewReconciler(cfg ReconcilerConfig) (*Reconciler, error) {
	reconciler, err := app.NewReconciler(store.New(cfg.DB, cfg.RuntimeOwner, cfg.Epoch, interlock.DurableReader{}), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("build reconciler: %w", err)
	}
	return &Reconciler{app: reconciler}, nil
}

// Resolve closes one command as succeeded, failed or manual_review with
// independent evidence, fenced by the runtime owner, and publishes the
// reconciliation notification.
func (r *Reconciler) Resolve(ctx context.Context, commandID, finalStatus string, evidence map[string]any) error {
	return r.app.Resolve(ctx, commandID, finalStatus, evidence) //nolint:wrapcheck // Facade operations must only delegate (architecture gate); the app layer owns the error context.
}

// Awaiting lists the tenant's commands awaiting reconciliation, oldest first.
func (r *Reconciler) Awaiting(ctx context.Context, tenantID string) ([]AwaitingCommand, error) {
	return r.app.Awaiting(ctx, tenantID) //nolint:wrapcheck // Facade operations must only delegate (architecture gate); the app layer owns the error context.
}
