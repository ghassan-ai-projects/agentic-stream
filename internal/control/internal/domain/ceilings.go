package domain

// CostCeilings is an operator's cost configuration. A nil field leaves the
// current value in place; the kill switch also applies to the tenant ceiling.
type CostCeilings struct {
	Global, Tenant *uint64
	KillSwitch     *bool
}

// Empty reports whether the configuration changes nothing.
func (c CostCeilings) Empty() bool {
	return c.Global == nil && c.Tenant == nil && c.KillSwitch == nil
}

// ChangesGlobal reports whether the global ceiling or kill switch is set.
func (c CostCeilings) ChangesGlobal() bool { return c.Global != nil || c.KillSwitch != nil }

// MergeGlobal keeps the current global value for anything left unset.
func (c CostCeilings) MergeGlobal(maxMicro uint64, killSwitch bool) (uint64, bool) {
	return valueOr(c.Global, maxMicro), valueOr(c.KillSwitch, killSwitch)
}

// TenantKillSwitch is the kill switch for the tenant ceiling; unset disables it.
func (c CostCeilings) TenantKillSwitch() bool { return valueOr(c.KillSwitch, false) }

func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
