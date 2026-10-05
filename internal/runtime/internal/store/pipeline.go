package store

import (
	"context"
	"database/sql"
	"fmt"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

// PipelineStore joins governance and owner checks to the caller-owned database.
type PipelineStore struct {
	DB         *storage.DB
	Policy     *policy.Service
	Owner      *runtimecontrol.RuntimeOwner
	OwnerEpoch string
}

// AssertOwner checks the bound owner epoch in the original transaction scope.
func (p *PipelineStore) AssertOwner(ctx context.Context) error {
	if p.Owner == nil || p.OwnerEpoch == "" {
		return nil
	}
	if err := p.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return p.Owner.Assert(ctx, tx, p.OwnerEpoch)
	}); err != nil {
		return fmt.Errorf("runtime ownership lost: %w", err)
	}
	return nil
}

// NextPendingIntent delegates the policy-owned queue projection.
func (p *PipelineStore) NextPendingIntent(ctx context.Context, tenant string) (string, bool, error) {
	return policy.NextPendingIntent(ctx, p.DB.DB, tenant) //nolint:wrapcheck // Policy names the failed read; preserve the existing batch error text.
}

// EvaluateIntent commits one policy evaluation on the original transaction.
func (p *PipelineStore) EvaluateIntent(ctx context.Context, intentID string, now time.Time) error {
	if err := p.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := p.Policy.EvaluateIntent(ctx, tx, policy.EvaluationRequest{IntentID: intentID, Now: now}); err != nil {
			return fmt.Errorf("evaluate intent: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("evaluate intent %s: %w", intentID, err)
	}
	return nil
}
