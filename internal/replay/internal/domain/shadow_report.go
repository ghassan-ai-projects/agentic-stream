package domain

import "encoding/json"

type ShadowReport struct {
	Mode            string             `json:"mode"`
	EventsProcessed int                `json:"events_processed"`
	VersionCount    int                `json:"situation_versions"`
	VersionsHash    string             `json:"versions_hash"`
	EffectsAllowed  bool               `json:"effects_allowed"`
	Comparisons     []ShadowReportItem `json:"comparisons"`
	Findings        []ReportFinding    `json:"findings"`
}

type ShadowReportItem struct {
	EpisodeKey        string          `json:"episode_key"`
	ComparisonSHA256  string          `json:"comparison_sha256"`
	DecisionsEqual    bool            `json:"decisions_equal"`
	Comparison        json.RawMessage `json:"comparison"`
	BaselineDecision  json.RawMessage `json:"baseline_decision"`
	CandidateDecision json.RawMessage `json:"candidate_decision"`
}

type ReportFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (r Result) DifferingComparisons() int {
	differing := 0
	for _, comparison := range r.ShadowComparisons {
		if !comparison.DecisionsEqual {
			differing++
		}
	}
	return differing
}

func (r Result) ShadowReport() ShadowReport {
	report := ShadowReport{
		Mode: string(r.Mode), EventsProcessed: r.EventsProcessed, VersionCount: r.VersionCount, VersionsHash: r.VersionsHash, EffectsAllowed: r.EffectsAllowed,
		Comparisons: []ShadowReportItem{}, Findings: []ReportFinding{},
	}
	for _, comparison := range r.ShadowComparisons {
		report.Comparisons = append(report.Comparisons, reportItem(comparison))
	}
	for _, finding := range r.Findings {
		report.Findings = append(report.Findings, ReportFinding(finding))
	}
	return report
}

func reportItem(c ShadowComparisonResult) ShadowReportItem {
	return ShadowReportItem{
		EpisodeKey: c.EpisodeKey, ComparisonSHA256: c.ComparisonSHA256, DecisionsEqual: c.DecisionsEqual,
		Comparison: c.ComparisonJSON, BaselineDecision: c.BaselineDecisionJSON, CandidateDecision: c.TamozDecisionJSON,
	}
}
