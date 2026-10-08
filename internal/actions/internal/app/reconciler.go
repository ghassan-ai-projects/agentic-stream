package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

type AwaitingCommand = domain.AwaitingCommand

type Reconciler struct{ service *Service }

func NewReconciler(st store.Store, clk sources.Clock, ids sources.Generator) (*Reconciler, error) {
	if !st.Configured() {
		return nil, errors.New("reconciliation requires a database and runtime owner check")
	}
	return &Reconciler{service: &Service{store: st, clk: sources.OrPhysical(clk), ids: sources.OrRandom(ids)}}, nil
}

func (r *Reconciler) Resolve(ctx context.Context, commandID, finalStatus string, evidence map[string]any) error {
	if err := r.service.reconcileUnknown(ctx, commandID, finalStatus, evidence); err != nil {
		return fmt.Errorf("resolve command %s as %s: %w", commandID, finalStatus, err)
	}
	return nil
}

func (r *Reconciler) Awaiting(ctx context.Context, tenantID string) ([]domain.AwaitingCommand, error) {
	awaiting, err := r.service.store.AwaitingReconciliation(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("commands awaiting reconciliation for tenant %s: %w", tenantID, err)
	}
	return awaiting, nil
}
