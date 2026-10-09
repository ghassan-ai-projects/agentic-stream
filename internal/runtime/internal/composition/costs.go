package composition

import (
	"context"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
)

func configureCostLimits(ctx context.Context, cfg PipelineConfig) error {
	return store.ConfigureCostLimits(ctx, store.CostConfiguration{DB: cfg.DB, RuntimeOwner: runtimeOwnershipCheck(cfg), OwnerEpoch: cfg.OwnerEpoch, Clock: cfg.Clock, TenantID: cfg.TenantID, Ceilings: runtimecontrol.CostCeilings{Global: cfg.GlobalCostCeiling, Tenant: cfg.TenantCostCeiling, KillSwitch: cfg.CostKillSwitch}})
}
