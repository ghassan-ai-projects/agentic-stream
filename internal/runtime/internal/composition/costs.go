package composition

import (
	"context"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
)

func configureCostLimits(ctx context.Context, cfg PipelineConfig) error {
	return store.ConfigureCostLimits(ctx, store.CostConfiguration{DB: cfg.DB, Owner: cfg.Owner, OwnerEpoch: cfg.OwnerEpoch, Clock: cfg.Clock, TenantID: cfg.TenantID, Ceilings: costcontrol.Ceilings{Global: cfg.GlobalCostCeiling, Tenant: cfg.TenantCostCeiling, KillSwitch: cfg.CostKillSwitch}})
}
