package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Rules) judgeTrigger(tr spec.Trigger, in triggerInputs) (triggerVerdict, error) {
	fired, err := e.evalBool(tr, "when", in)
	if err != nil || !fired {
		return triggerVerdict{reason: "trigger condition false"}, err
	}
	score, err := e.evalScore(tr, in)
	if err != nil {
		return triggerVerdict{}, err
	}
	if reason, met := completenessMet(tr, in.completeness); !met {
		return triggerVerdict{score: score, reason: reason}, nil
	}
	if material, err := e.materialDelta(tr, in); err != nil || !material {
		return triggerVerdict{score: score, reason: "material delta false"}, err
	}
	return thresholdVerdict(tr, score), nil
}

func completenessMet(tr spec.Trigger, completeness string) (string, bool) {
	if tr.Completeness == "" || tr.Completeness == "any" {
		return "", true
	}
	if operators.Completeness(completeness).AtLeast(operators.Completeness(tr.Completeness)) {
		return "", true
	}
	return fmt.Sprintf("completeness %s does not meet %s", completeness, tr.Completeness), false
}

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
