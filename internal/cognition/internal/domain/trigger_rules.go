package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// judgeTrigger admits a trigger whose condition holds, whose delta is material, and
// whose score meets the threshold, and otherwise names the first gate that
// failed.
func (e *Rules) judgeTrigger(tr spec.Trigger, in triggerInputs) (triggerVerdict, error) {
	fired, err := e.evalBool(tr, "when", in)
	if err != nil || !fired {
		return triggerVerdict{reason: "trigger condition false"}, err
	}
	score, err := e.evalScore(tr, in)
	if err != nil {
		return triggerVerdict{}, err
	}
	if material, err := e.materialDelta(tr, in); err != nil || !material {
		return triggerVerdict{score: score, reason: "material delta false"}, err
	}
	return thresholdVerdict(tr, score), nil
}

// materialDelta holds when the trigger has no material-delta condition.
func (e *Rules) materialDelta(tr spec.Trigger, in triggerInputs) (bool, error) {
	if tr.MaterialDelta == "" {
		return true, nil
	}
	return e.evalBool(tr, "materialDelta", in)
}

func thresholdVerdict(tr spec.Trigger, score float64) triggerVerdict {
	if score < tr.Threshold {
		return triggerVerdict{score: score, reason: fmt.Sprintf("score %.2f below threshold %.2f", score, tr.Threshold)}
	}
	return triggerVerdict{score: score, reason: fmt.Sprintf("score %.2f meets threshold %.2f", score, tr.Threshold), admitted: true}
}
