package app

import (
	"context"
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// AwaitingCommand is a command whose outcome is uncertain.
type AwaitingCommand = domain.AwaitingCommand

// Reconciler closes commands whose outcome is uncertain from independent
// evidence an operator supplies. It never dispatches, so it needs no effector;
// every resolution is owner-fenced and recorded like an automatic one.
type Reconciler struct{ service *Service }

// NewReconciler requires a fully configured store.
func NewReconciler(st store.Store, clk sources.Clock, ids sources.Generator) (*Reconciler, error) {
	if !st.Configured() {
		return nil, errors.New("reconciliation requires a database, runtime owner check and interlock")
	}
	return &Reconciler{service: &Service{store: st, clk: orPhysical(clk), ids: orRandom(ids)}}, nil
}

// Resolve closes one command as finalStatus with the evidence.
func (r *Reconciler) Resolve(ctx context.Context, commandID, finalStatus string, evidence map[string]any) error {
	return r.service.reconcileUnknown(ctx, commandID, finalStatus, evidence)
}

// Awaiting lists the tenant's commands awaiting reconciliation.
func (r *Reconciler) Awaiting(ctx context.Context, tenantID string) ([]domain.AwaitingCommand, error) {
	return r.service.store.AwaitingReconciliation(ctx, tenantID) //nolint:wrapcheck // The store names the failed read.
}
