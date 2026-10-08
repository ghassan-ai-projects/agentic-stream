package domain

import (
	"reflect"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Rules) buildFeatures(v situations.Version) map[string]any {
	evidence := v.Evidence
	if evidence == nil {
		evidence = []string{}
	}
	return situations.CELFeatures(e.spec, v.Facts, evidence)
}

func (e *Rules) buildSituation(v situations.Version) map[string]any {
	return map[string]any{
		"phase":       v.Phase,
		"severity":    v.Severity,
		"confidence":  v.Confidence,
		"uncertainty": 1.0 - v.Confidence,
		"entity": map[string]any{
			"type": v.EntityType,
			"id":   v.EntityID,
		},
	}
}

func (e *Rules) buildDelta(current situations.Version, previous *situations.Version) map[string]any {
	if previous == nil {
		return firstVersionDelta(current)
	}
	prevFacts := previous.Facts
	if prevFacts == nil {
		prevFacts = map[string]any{}
	}
	return changeDelta(current, *previous, prevFacts)
}

// changeDelta describes what changed since the previously reasoned version.
func changeDelta(current, previous situations.Version, prevFacts map[string]any) map[string]any {
	factsChanged := !mapsEqual(prevFacts, current.Facts)
	phaseChanged := current.Phase != previous.Phase
	return map[string]any{
		spec.DeltaKeys.PhaseChanged: phaseChanged, spec.DeltaKeys.SeverityChange: current.Severity - previous.Severity,
		spec.DeltaKeys.CompletenessChanged: current.Completeness != previous.Completeness,
		// A Situation carries no hypothesis: hypotheses are Decision output, and
		// feeding them back into Situation state would make history depend on
		// recorded cognition. The key is true for a first reasoned version and
		// false after it, so specs can reference it deterministically.
		spec.DeltaKeys.PrimaryHypothesisChanged: false, spec.DeltaKeys.FactsChanged: factsChanged,
		spec.DeltaKeys.Facts: prevFacts, spec.DeltaKeys.NewFacts: current.Facts,
		spec.DeltaKeys.Novelty: novelty(phaseChanged || factsChanged),
	}
}

// firstVersionDelta treats every aspect of a first reasoned version as new.
func firstVersionDelta(current situations.Version) map[string]any {
	return map[string]any{
		spec.DeltaKeys.PhaseChanged: true, spec.DeltaKeys.SeverityChange: current.Severity,
		spec.DeltaKeys.CompletenessChanged: true, spec.DeltaKeys.PrimaryHypothesisChanged: true,
		spec.DeltaKeys.FactsChanged: true, spec.DeltaKeys.Facts: map[string]any{},
		spec.DeltaKeys.NewFacts: current.Facts, spec.DeltaKeys.Novelty: 1.0,
	}
}

func novelty(changed bool) float64 {
	if changed {
		return 1.0
	}
	return 0.0
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(va, vb) {
			return false
		}
	}
	return true
}
