package cognition

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Engine) evaluate(ctx context.Context, tr spec.Trigger, current situations.Version, previous *situations.Version) (Evaluation, error) {
	eval := e.newEvaluation(tr, current)
	inputs := e.triggerInputs(current, previous)
	deltaJSON, err := canonicaljson.Marshal(inputs.delta)
	if err != nil {
		return eval, fmt.Errorf("marshal delta: %w", err)
	}
	eval.DeltaJSON = deltaJSON
	verdict, err := e.judgeTrigger(ctx, tr, inputs)
	eval.Score = verdict.score
	if err != nil {
		return eval, err
	}
	eval.recordVerdict(verdict)
	return eval, nil
}

// newEvaluation starts an ignored evaluation of the trigger against the
// version, stamped with the current time.
func (e *Engine) newEvaluation(tr spec.Trigger, current situations.Version) Evaluation {
	evalAt := e.clk.Now().UTC()
	return Evaluation{
		TriggerID: e.triggerID(tr.Name, current.SituationID, current.Version), TriggerName: tr.Name,
		SituationID: current.SituationID, SituationVersion: current.Version,
		Threshold: tr.Threshold, Lane: tr.Lane, Outcome: "ignored",
		PolicySHA256: e.spec.Digest, EvaluatedAt: evalAt,
	}
}

func (e *Engine) triggerInputs(current situations.Version, previous *situations.Version) triggerInputs {
	return triggerInputs{
		features: e.buildFeatures(current), situation: e.buildSituation(current),
		delta: e.buildDelta(current, previous), eventHorizon: current.EventHorizon, watermark: current.Watermark,
	}
}

func (eval *Evaluation) recordVerdict(verdict triggerVerdict) {
	eval.Reasons = append(eval.Reasons, verdict.reason)
	if verdict.admitted {
		eval.Outcome = "admitted"
	}
}

// triggerInputs are the CEL inputs one trigger is evaluated over.
type triggerInputs struct {
	features, situation, delta map[string]any
	eventHorizon, watermark    time.Time
}

// triggerVerdict is the outcome of one trigger evaluation and its reason.
type triggerVerdict struct {
	score    float64
	reason   string
	admitted bool
}

// judgeTrigger admits a trigger whose condition holds, whose delta is material, and
// whose score meets the threshold, and otherwise names the first gate that
// failed.
func (e *Engine) judgeTrigger(_ context.Context, tr spec.Trigger, in triggerInputs) (triggerVerdict, error) {
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
func (e *Engine) materialDelta(tr spec.Trigger, in triggerInputs) (bool, error) {
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
