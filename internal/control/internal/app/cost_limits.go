package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// SetLimit configures a ceiling or kill switch. It is intended to run inside an
// owner-fenced transaction.
func SetLimit(ctx context.Context, tx *store.Tx, scopeKey, tenantID string, maxMicro uint64, killSwitch bool, now string) error {
	if err := domain.CheckLimit(scopeKey, maxMicro, now); err != nil {
		return err
	}
	rows, err := tx.WriteLimit(ctx, scopeKey, tenantID, domain.Micro(maxMicro), killSwitch, now)
	if err != nil {
		return err
	}
	return domain.CheckLimitWritten(rows)
}

// ApplyCeilings writes the global and tenant ceilings inside the caller's
// owner-fenced transaction.
func ApplyCeilings(ctx context.Context, tx *store.Tx, ceilings domain.CostCeilings, tenantID, now string) error {
	if err := applyGlobalCeiling(ctx, tx, ceilings, now); err != nil {
		return err
	}
	return applyTenantCeiling(ctx, tx, ceilings, tenantID, now)
}

// applyGlobalCeiling updates the global ceiling and kill switch, keeping any
// value the configuration leaves unset.
func applyGlobalCeiling(ctx context.Context, tx *store.Tx, ceilings domain.CostCeilings, now string) error {
	if !ceilings.ChangesGlobal() {
		return nil
	}
	current, kill, err := readLimit(ctx, tx, domain.GlobalScope)
	if err != nil {
		return fmt.Errorf("read global cost limit: %w", err)
	}
	maxMicro, kill := ceilings.MergeGlobal(current, kill)
	if err := SetLimit(ctx, tx, domain.GlobalScope, "", maxMicro, kill, now); err != nil {
		return fmt.Errorf("set global cost limit: %w", err)
	}
	return nil
}

// applyTenantCeiling sets the tenant ceiling; an unset kill switch disables it.
func applyTenantCeiling(ctx context.Context, tx *store.Tx, ceilings domain.CostCeilings, tenantID, now string) error {
	if ceilings.Tenant == nil {
		return nil
	}
	if err := SetLimit(ctx, tx, domain.TenantScope(tenantID), tenantID, *ceilings.Tenant, ceilings.TenantKillSwitch(), now); err != nil {
		return fmt.Errorf("set tenant cost limit: %w", err)
	}
	return nil
}

func readLimit(ctx context.Context, tx *store.Tx, scopeKey string) (uint64, bool, error) {
	maxMicro, kill, err := tx.ReadLimit(ctx, scopeKey)
	if err != nil {
		return 0, false, err
	}
	value, err := domain.CheckStoredLimit(scopeKey, maxMicro)
	return value, kill, err
}
