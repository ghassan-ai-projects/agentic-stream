package domain

import (
	"math"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	CauseOutOfCap = "out_of_cap"

	OutOfCapWindow     = 24 * time.Hour
	outOfCapMinMatches = 4
)

func ProposedEnvelope(caseFeatures []map[string]any, names []string, margin float64) (map[string]spec.Range, map[string]spec.Range) {
	envelope, limits := map[string]spec.Range{}, map[string]spec.Range{}
	for _, name := range names {
		bounds, observed := observedRange(caseFeatures, name)
		if !observed {
			continue
		}
		envelope[name], limits[name] = bounds, widened(bounds, margin)
	}
	return envelope, limits
}

func observedRange(caseFeatures []map[string]any, name string) (spec.Range, bool) {
	var values []float64
	for _, features := range caseFeatures {
		if value, numeric := number(features[name]); numeric {
			values = append(values, value)
		}
	}
	if len(values) == 0 || len(values) != len(caseFeatures) {
		return spec.Range{}, false
	}
	return spec.Range{Min: slices.Min(values), Max: slices.Max(values)}, true
}

func widened(bounds spec.Range, margin float64) spec.Range {
	pad := margin * (bounds.Max - bounds.Min)
	if pad == 0 {
		pad = margin * math.Max(math.Abs(bounds.Min), math.Abs(bounds.Max))
	}
	return spec.Range{Min: bounds.Min - pad, Max: bounds.Max + pad}
}

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
