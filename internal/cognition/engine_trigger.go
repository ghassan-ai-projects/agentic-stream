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
	evalAt := e.clk.Now().UTC()
	eval := Evaluation{
		TriggerID:        e.triggerID(tr.Name, current.SituationID, current.Version),
		TriggerName:      tr.Name,
		SituationID:      current.SituationID,
		SituationVersion: current.Version,
		Threshold:        tr.Threshold,
		Lane:             tr.Lane,
		Outcome:          "ignored",
		PolicySHA256:     e.spec.Digest,
		EvaluatedAt:      evalAt,
	}
	inputs := triggerInputs{
		features: e.buildFeatures(current), situation: e.buildSituation(current),
		delta: e.buildDelta(current, previous), eventHorizon: current.EventHorizon, watermark: current.Watermark,
	}
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
	eval.Reasons = append(eval.Reasons, verdict.reason)
	if verdict.admitted {
		eval.Outcome = "admitted"
	}
	return eval, nil
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
func (e *Engine) judgeTrigger(ctx context.Context, tr spec.Trigger, in triggerInputs) (triggerVerdict, error) {
	fired, err := e.evalBool(ctx, tr, "when", in.features, in.situation, in.delta, in.eventHorizon, in.watermark)
	if err != nil || !fired {
		return triggerVerdict{reason: "trigger condition false"}, err
	}
	score, err := e.evalScore(ctx, tr, in.features, in.situation, in.delta, in.eventHorizon, in.watermark)
	if err != nil {
		return triggerVerdict{}, err
	}
	if tr.MaterialDelta != "" {
		material, err := e.evalBool(ctx, tr, "materialDelta", in.features, in.situation, in.delta, in.eventHorizon, in.watermark)
		if err != nil || !material {
			return triggerVerdict{score: score, reason: "material delta false"}, err
		}
	}
	if score < tr.Threshold {
		return triggerVerdict{score: score, reason: fmt.Sprintf("score %.2f below threshold %.2f", score, tr.Threshold)}, nil
	}
	return triggerVerdict{score: score, reason: fmt.Sprintf("score %.2f meets threshold %.2f", score, tr.Threshold), admitted: true}, nil
}
