package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Rules) Material(current situations.Version, previous *situations.Version) bool {
	if previous == nil || current.Phase != previous.Phase || e.endsOccurrence(current.Phase) || e.endsOccurrence(previous.Phase) {
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
		if e.declaresMaterial(trigger, inputs) {
			return true
		}
	}
	return false
}

func (e *Rules) declaresMaterial(trigger spec.Trigger, inputs triggerInputs) bool {
	if trigger.MaterialDelta == "" {
		return true
	}
	if applies, err := e.evalBool(trigger, "when", inputs); err == nil && !applies {
		return false
	}
	material, err := e.materialDelta(trigger, inputs)
	return err != nil || material
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
