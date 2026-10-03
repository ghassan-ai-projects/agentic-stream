package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
)

// configureCostLimits applies the operator's cost ceilings under the runtime
// owner's fence.
func configureCostLimits(ctx context.Context, cfg PipelineConfig) error {
	ceilings := costcontrol.Ceilings{Global: cfg.GlobalCostCeiling, Tenant: cfg.TenantCostCeiling, KillSwitch: cfg.CostKillSwitch}
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

func assertCostConfigurationOwner(ctx context.Context, tx *sql.Tx, cfg PipelineConfig) error {
	if cfg.Owner != nil && cfg.OwnerEpoch != "" {
		if err := cfg.Owner.Assert(ctx, tx, cfg.OwnerEpoch); err != nil {
			return fmt.Errorf("assert owner for cost configuration: %w", err)
		}
	}
	return nil
}
