package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

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
