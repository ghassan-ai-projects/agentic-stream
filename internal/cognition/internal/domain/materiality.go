package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

// Material reports whether a version changes its Situation materially against
// the previously evaluated version (ADR-018). A newer version that is not
// material leaves pending intents fresh. The spec declares materiality per
// trigger through materialDelta, evaluated whether or not the trigger fires;
// a first version, a version that ends the occurrence or follows its end, a
// spec without triggers, a trigger without materialDelta and a materialDelta
// that fails to evaluate are all material, so the rule fails strict.
func (e *Rules) Material(current situations.Version, previous *situations.Version) bool {
	if previous == nil || e.endsOccurrence(current.Phase) || e.endsOccurrence(previous.Phase) {
		return true
	}
	return e.anyTriggerDeclaresMaterial(current, previous)
}

func (e *Rules) anyTriggerDeclaresMaterial(current situations.Version, previous *situations.Version) bool {
	if len(e.spec.Cognition.Triggers) == 0 {
		return true
	}
	inputs := e.triggerInputs(current, previous)
	for _, trigger := range e.spec.Cognition.Triggers {
		if material, err := e.materialDelta(trigger, inputs); err != nil || material {
			return true
		}
	}
	return false
}

// endsOccurrence reports the closing phase and any phase the spec marks
// terminal.
func (e *Rules) endsOccurrence(phase string) bool {
	if phase == situations.PhaseResolved {
		return true
	}
	for _, declared := range e.spec.Situation.Phases {
		if declared.Name == phase {
			return declared.Terminal
		}
	}
	return false
}
