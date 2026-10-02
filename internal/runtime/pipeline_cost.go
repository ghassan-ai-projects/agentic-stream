package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"time"
)

func configureCostLimits(ctx context.Context, cfg PipelineConfig) error {
	if cfg.GlobalCostCeiling == nil && cfg.TenantCostCeiling == nil && cfg.CostKillSwitch == nil {
		return nil
	}
	if err := cfg.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := assertCostConfigurationOwner(ctx, tx, cfg); err != nil {
			return err
		}
		if err := configureGlobalCostLimit(ctx, tx, cfg); err != nil {
			return err
		}
		return configureTenantCostLimit(ctx, tx, cfg)
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

func configureGlobalCostLimit(ctx context.Context, tx *sql.Tx, cfg PipelineConfig) error {
	if cfg.GlobalCostCeiling == nil && cfg.CostKillSwitch == nil {
		return nil
	}
	maxMicro, existingKill, err := readCostLimit(ctx, tx, "global")
	if err != nil {
		return fmt.Errorf("read global cost limit: %w", err)
	}
	if cfg.GlobalCostCeiling != nil {
		maxMicro = *cfg.GlobalCostCeiling
	}
	kill := existingKill
	if cfg.CostKillSwitch != nil {
		kill = *cfg.CostKillSwitch
	}
	if err := costcontrol.SetLimit(ctx, tx, "global", "", maxMicro, kill, cfg.Clock.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("set global cost limit: %w", err)
	}
	return nil
}

func configureTenantCostLimit(ctx context.Context, tx *sql.Tx, cfg PipelineConfig) error {
	if cfg.TenantCostCeiling == nil {
		return nil
	}
	kill := false
	if cfg.CostKillSwitch != nil {
		kill = *cfg.CostKillSwitch
	}
	if err := costcontrol.SetLimit(ctx, tx, "tenant:"+cfg.TenantID, cfg.TenantID, *cfg.TenantCostCeiling, kill, cfg.Clock.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("set tenant cost limit: %w", err)
	}
	return nil
}

func readCostLimit(ctx context.Context, tx *sql.Tx, scopeKey string) (uint64, bool, error) {
	var maxMicro int64
	var kill int
	if err := tx.QueryRowContext(ctx, "SELECT max_micro, kill_switch FROM cost_limits WHERE scope_key = ?", scopeKey).Scan(&maxMicro, &kill); err != nil {
		return 0, false, fmt.Errorf("load %s: %w", scopeKey, err)
	}
	if maxMicro < 0 {
		return 0, false, fmt.Errorf("cost limit %s is negative", scopeKey)
	}
	return uint64(maxMicro), kill != 0, nil
}
