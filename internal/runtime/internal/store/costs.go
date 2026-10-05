package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

// CostConfiguration binds operator ceilings to an original owner-fenced transaction.
type CostConfiguration struct {
	DB                   *storage.DB
	Owner                *runtimecontrol.RuntimeOwner
	OwnerEpoch, TenantID string
	Clock                clock.Clock
	Ceilings             costcontrol.Ceilings
}

// configureCostLimits applies the operator's cost ceilings under the runtime
// owner's fence.
func ConfigureCostLimits(ctx context.Context, cfg CostConfiguration) error {
	ceilings := cfg.Ceilings
	if ceilings.Empty() {
		return nil
	}
	if err := cfg.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := assertCostConfigurationOwner(ctx, tx, cfg); err != nil {
			return err
		}
		now := cfg.Clock.Now().UTC().Format(time.RFC3339Nano)
		return costcontrol.ApplyCeilings(ctx, tx, ceilings, cfg.TenantID, now) //nolint:wrapcheck // Wrapped below with the configuration step.
	}); err != nil {
		return fmt.Errorf("configure cost limits: %w", err)
	}
	return nil
}

func assertCostConfigurationOwner(ctx context.Context, tx *sql.Tx, cfg CostConfiguration) error {
	if cfg.Owner != nil && cfg.OwnerEpoch != "" {
		if err := cfg.Owner.Assert(ctx, tx, cfg.OwnerEpoch); err != nil {
			return fmt.Errorf("assert owner for cost configuration: %w", err)
		}
	}
	return nil
}
