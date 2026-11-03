package domain

import (
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const DefaultDraftAfter = 3

type Key struct {
	DiagnosisCode string
	KindDigest    string
	Intent        string
	Preset        string
}

type KeyedCase struct {
	Key
	EpisodeID string
	Verdict   string
	Features  map[string]any
	Summary   string
}

type KeyHistory struct {
	Pending          bool
	RejectedEvidence int
	Rejected         bool
}

func Eligible(key Key, cases []KeyedCase, history KeyHistory, draftAfter int, ruled bool) bool {
	if ruled || history.Pending || key.KindDigest == "" || key.DiagnosisCode == "" || key.DiagnosisCode == spec.UnknownDiagnosis || key.Intent == "" {
		return false
	}
	verified := VerifiedCount(key, cases)
	if verified < draftAfter || contradicted(key, cases) {
		return false
	}
	return !history.Rejected || verified > history.RejectedEvidence
}

func VerifiedCount(key Key, cases []KeyedCase) int {
	verified := 0
	for _, c := range cases {
		if c.Key == key && c.Verdict == CaseVerified {
			verified++
		}
	}
	return verified
}

func contradicted(key Key, cases []KeyedCase) bool {
	return slices.ContainsFunc(cases, func(c KeyedCase) bool {
		sameCode := c.DiagnosisCode == key.DiagnosisCode && c.KindDigest == key.KindDigest
		otherAnswer := c.Intent != key.Intent || c.Preset != key.Preset
		return sameCode && ((c.Verdict == CaseVerified && otherAnswer) || (c.Verdict == CaseNotVerified && !otherAnswer))
	})
}

func Keys(cases []KeyedCase) []Key {
	var keys []Key
	for _, c := range cases {
		if c.Verdict == CaseVerified && !slices.Contains(keys, c.Key) {
			keys = append(keys, c.Key)
		}
	}
	return keys
}

func (r *Rules) Rules(key Key) []spec.Rule {
	var same []spec.Rule
	for _, compiled := range r.compiled {
		if compiled.rule.DiagnosisCode == key.DiagnosisCode {
			same = append(same, compiled.rule)
		}
	}
	return same
}

func (r *Rules) Ruled(key Key) bool {
	return slices.ContainsFunc(r.Rules(key), func(rule spec.Rule) bool { return keyOfRule(rule) == key })
}

func keyOfRule(rule spec.Rule) Key {
	return Key{DiagnosisCode: rule.DiagnosisCode, KindDigest: rule.PinnedKind.Digest, Intent: rule.Recommend.Intent, Preset: rule.Recommend.Preset}
}
