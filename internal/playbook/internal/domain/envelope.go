package domain

import (
	"time"
)

const (
	CauseOutOfCap = "out_of_cap"

	OutOfCapWindow     = 24 * time.Hour
	outOfCapMinMatches = 4
)

type CapCounts struct {
	Matches    int
	OutsideCap int
}

func (c CapCounts) MostlyOutside() bool {
	return c.Matches >= outOfCapMinMatches && 2*c.OutsideCap > c.Matches
}

func OutOfCapCircuit(tenantID string, rule RuleKey, counts CapCounts, now time.Time) Circuit {
	return Circuit{Rule: rule, TenantID: tenantID, Cause: CauseOutOfCap, OpenedAt: now,
		Evidence: map[string]any{"matches": counts.Matches, "outside_cap": counts.OutsideCap, "window": OutOfCapWindow.String()}}
}
