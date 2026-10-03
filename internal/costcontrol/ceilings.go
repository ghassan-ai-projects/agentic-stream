package costcontrol

import (
	"context"
	"database/sql"
	"fmt"
)

// Ceilings is an operator's cost configuration. A nil field leaves the
// current value in place; the kill switch also applies to the tenant ceiling.
type Ceilings struct {
	Global, Tenant *uint64
	KillSwitch     *bool
}

// Empty reports whether the configuration changes nothing.
func (c Ceilings) Empty() bool {
	return c.Global == nil && c.Tenant == nil && c.KillSwitch == nil
}

// ApplyCeilings writes the global and tenant ceilings inside the caller's
// owner-fenced transaction.
func ApplyCeilings(ctx context.Context, tx *sql.Tx, ceilings Ceilings, tenantID, now string) error {
	if err := applyGlobalCeiling(ctx, tx, ceilings, now); err != nil {
		return err
	}
	return applyTenantCeiling(ctx, tx, ceilings, tenantID, now)
}

// applyGlobalCeiling updates the global ceiling and kill switch, keeping any
// value the configuration leaves unset.
func applyGlobalCeiling(ctx context.Context, tx *sql.Tx, ceilings Ceilings, now string) error {
	if ceilings.Global == nil && ceilings.KillSwitch == nil {
		return nil
	}
	maxMicro, kill, err := readCostLimit(ctx, tx, "global")
	if err != nil {
		return fmt.Errorf("read global cost limit: %w", err)
	}
	maxMicro, kill = valueOr(ceilings.Global, maxMicro), valueOr(ceilings.KillSwitch, kill)
	if err := SetLimit(ctx, tx, "global", "", maxMicro, kill, now); err != nil {
		return fmt.Errorf("set global cost limit: %w", err)
	}
	return nil
}

// applyTenantCeiling sets the tenant ceiling; an unset kill switch disables it.
func applyTenantCeiling(ctx context.Context, tx *sql.Tx, ceilings Ceilings, tenantID, now string) error {
	if ceilings.Tenant == nil {
		return nil
	}
	kill := valueOr(ceilings.KillSwitch, false)
	if err := SetLimit(ctx, tx, "tenant:"+tenantID, tenantID, *ceilings.Tenant, kill, now); err != nil {
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

func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
