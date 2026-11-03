package domain

import (
	"maps"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	VerdictPending      = "pending"
	VerdictAgree        = "agree"
	VerdictDisagree     = "disagree"
	VerdictInconclusive = "inconclusive"
)

type Recommendation struct {
	RuleID         string         `json:"rule_id"`
	RuleVersion    int            `json:"rule_version"`
	PlaybookDigest string         `json:"playbook_digest"`
	DiagnosisCode  string         `json:"diagnosis_code"`
	Intent         string         `json:"intent"`
	Preset         string         `json:"preset"`
	Parameters     map[string]any `json:"parameters"`
	Features       map[string]any `json:"features"`
}

func (r *Rules) Recommend(match Match) Recommendation {
	return Recommendation{
		RuleID: match.Rule.ID, RuleVersion: match.Rule.Version, PlaybookDigest: r.digest,
		DiagnosisCode: match.Rule.DiagnosisCode, Intent: match.Rule.Recommend.Intent, Preset: match.Rule.Recommend.Preset,
		Parameters: presetParameters(r.spec, match.Rule.Recommend), Features: match.Features,
	}
}

func presetParameters(compiled *spec.CompiledSpec, recommend spec.Recommendation) map[string]any {
	intent, _ := compiled.Intent(recommend.Intent)
	return maps.Clone(intent.Presets[recommend.Preset])
}

func (r Recommendation) Key() RuleKey {
	return RuleKey{ID: r.RuleID, Version: r.RuleVersion}
}

func (r Recommendation) Verdict(observed Observed) string {
	if observed.DiagnosisCode == r.DiagnosisCode && observed.Intent == r.Intent && observed.Preset == r.Preset {
		return VerdictAgree
	}
	return VerdictDisagree
}
