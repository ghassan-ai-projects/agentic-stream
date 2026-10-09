package domain

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Engine) evaluate(ctx context.Context, sit *Situation, watermark time.Time, completenessChanged bool) (*Version, error) {
	inputs := evaluationInputs{features: e.buildFeaturesMap(sit), situation: e.buildSituationMap(sit), eventTime: sit.LatestEventTime, watermark: watermark}
	publishedPhase := sit.Phase
	changed, err := e.advanceLifecycle(ctx, sit, inputs)
	if err != nil {
		return nil, err
	}
	if !changed && (!completenessChanged || sit.Version == 0) {
		return nil, nil
	}
	return e.publish(sit, publishedPhase, watermark)
}

func (e *Engine) publish(sit *Situation, publishedPhase string, watermark time.Time) (*Version, error) {
	if sit.Phase != publishedPhase {
		sit.PreviousPhase = publishedPhase
	}
	sit.Version++
	sit.UpdatedAt = watermark
	return e.materialize(sit, watermark)
}

func (e *Engine) advanceLifecycle(ctx context.Context, sit *Situation, inputs evaluationInputs) (bool, error) {
	closed, err := e.closeOccurrence(ctx, sit, inputs)
	if err != nil {
		return false, err
	}
	transitioned, err := e.applyTransitions(ctx, sit, inputs)
	if err != nil {
		return false, err
	}
	opened, err := e.openOccurrence(ctx, sit, inputs)
	if err != nil {
		return false, err
	}
	return closed || transitioned || opened, nil
}

type evaluationInputs struct {
	features, situation map[string]any
	eventTime           time.Time
	watermark           time.Time
}

const PhaseResolved = "resolved"

func (e *Engine) closeOccurrence(ctx context.Context, sit *Situation, in evaluationInputs) (bool, error) {
	if e.closesThroughTransitions() || !e.occurrenceOpen(sit) {
		return false, nil
	}
	closed, err := e.evalBool(ctx, e.spec.Situation.Occurrence.CloseWhen, in.features, in.situation)
	if err != nil || !closed {
		return false, err
	}
	return e.transition(sit, PhaseResolved, in), nil
}

func (e *Engine) closesThroughTransitions() bool {
	return slices.ContainsFunc(e.spec.Situation.Transitions, func(tr spec.Transition) bool { return tr.To == PhaseResolved })
}

func (e *Engine) occurrenceOpen(sit *Situation) bool {
	notYetOpened := sit.Version == 0 && sit.Phase == e.spec.Situation.InitialPhase
	return !notYetOpened && sit.Phase != PhaseResolved
}

func (e *Engine) applyTransitions(ctx context.Context, sit *Situation, in evaluationInputs) (bool, error) {
	changed := false
	for _, tr := range e.spec.Situation.Transitions {
		if tr.From != sit.Phase {
			continue
		}
		moved, err := e.applyTransition(ctx, sit, tr, in)
		if err != nil {
			return false, err
		}
		changed = changed || moved
	}
	return changed, nil
}

func (e *Engine) applyTransition(ctx context.Context, sit *Situation, tr spec.Transition, in evaluationInputs) (bool, error) {
	held, err := e.evalBool(ctx, tr.When, in.features, in.situation)
	if err != nil {
		return false, err
	}
	key := tr.From + "->" + tr.To
	if !held {
		delete(sit.ConditionStart, key)
		return false, nil
	}
	minDuration, err := optionalDuration(tr.MinDuration)
	if err != nil {
		return false, fmt.Errorf("transition %s minDuration: %w", key, err)
	}
	start := conditionStart(sit, key, in.eventTime)
	return in.eventTime.Sub(start) >= minDuration && e.transition(sit, tr.To, in), nil
}

func optionalDuration(text string) (time.Duration, error) {
	if text == "" {
		return 0, nil
	}
	duration, err := spec.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return duration, nil
}

func conditionStart(sit *Situation, key string, eventTime time.Time) time.Time {
	start := sit.ConditionStart[key]
	if start.IsZero() {
		start = eventTime
		sit.ConditionStart[key] = start
	}
	return start
}

func (e *Engine) openOccurrence(ctx context.Context, sit *Situation, in evaluationInputs) (bool, error) {
	switch {
	case sit.Version == 0 && sit.Phase == e.spec.Situation.InitialPhase:
		return e.evalBool(ctx, e.spec.Situation.Occurrence.OpenWhen, in.features, in.situation)
	case sit.Phase == PhaseResolved:
		return e.reopenOccurrence(ctx, sit, in)
	}
	return false, nil
}

func (e *Engine) reopenOccurrence(ctx context.Context, sit *Situation, in evaluationInputs) (bool, error) {
	cooldown, err := optionalDuration(e.spec.Situation.Occurrence.ReopenCooldown)
	if err != nil {
		return false, fmt.Errorf("occurrence reopenCooldown: %w", err)
	}
	if in.eventTime.Sub(sit.ResolvedAt) < cooldown {
		return false, nil
	}
	opened, err := e.evalBool(ctx, e.spec.Situation.Occurrence.OpenWhen, in.features, in.situation)
	if err != nil || !opened {
		return false, err
	}
	e.startNextOccurrence(sit, in.eventTime)
	return true, nil
}

func (e *Engine) startNextOccurrence(sit *Situation, eventTime time.Time) {
	sit.OccurrenceID = nextOccurrenceID(sit.SituationID, sit.Version+1)
	sit.Phase = e.spec.Situation.InitialPhase
	sit.Severity = e.initialSeverity()
	sit.FirstEventTime, sit.OpenedAt = eventTime, eventTime
	sit.ResolvedAt = time.Time{}
	sit.ConditionStart = make(map[string]time.Time)
}

func nextOccurrenceID(situationID string, version int) string {
	identity := fmt.Sprintf("%s\x00occurrence\x00%d", situationID, version)
	return "occ_" + hex.EncodeToString(canonicaljson.Sum([]byte(identity)))
}

func (e *Engine) transition(sit *Situation, to string, in evaluationInputs) bool {
	if sit.Phase == to {
		return false
	}
	sit.Phase = to
	sit.Severity = e.severityForPhase(to)
	sit.ConditionStart = make(map[string]time.Time)
	if to == PhaseResolved {
		sit.ResolvedAt = in.eventTime
	}
	return true
}

func (e *Engine) severityForPhase(phase string) int {
	for _, p := range e.spec.Situation.Phases {
		if p.Name == phase {
			return p.Severity
		}
	}
	return 0
}

func (e *Engine) buildFeaturesMap(sit *Situation) map[string]any {
	return CELFeatures(e.spec, sit.Facts, sortedEvidenceIDs(sit))
}

func CELFeatures(compiled *spec.CompiledSpec, facts map[string]any, evidence []string) map[string]any {
	features := make(map[string]any)
	for _, r := range compiled.Situation.Reducers {
		addReducedFeature(features, facts, evidence, r)
	}
	addOperatorDefaults(features, compiled.Operators)
	return features
}

func addReducedFeature(features, facts map[string]any, evidence []string, r spec.Reducer) {
	switch r.Strategy {
	case "latest_event_time":

		if v, ok := facts[r.Field]; ok && v != nil {
			features[r.Input] = v
		}
	case "set_union":
		features[r.Field] = evidence
	}
}

func addOperatorDefaults(features map[string]any, ops []spec.Operator) {
	for _, op := range ops {
		if _, ok := features[op.Output]; ok {
			continue
		}
		if op.Kind == "missing_heartbeat" {
			features[op.Output] = false
		} else {
			features[op.Output] = 0.0
		}
	}
}

func (e *Engine) buildSituationMap(sit *Situation) map[string]any {
	return map[string]any{
		"phase":      sit.Phase,
		"severity":   sit.Severity,
		"confidence": sit.Confidence,
		"entity": map[string]any{
			"type": sit.EntityType,
			"id":   sit.EntityID,
		},
	}
}

func (e *Engine) evalBool(_ context.Context, expr string, features, situation map[string]any) (bool, error) {
	if expr == "" {
		return false, nil
	}
	prg, err := e.program(expr)
	if err != nil {
		return false, err
	}
	out, _, err := prg.Eval(map[string]any{"features": features, "situation": situation})
	if err != nil {
		return false, fmt.Errorf("eval cel: %w", err)
	}
	return spec.CELBool(out) //nolint:wrapcheck // The spec helper names the conversion failure.
}

func (e *Engine) program(expr string) (cel.Program, error) {
	ast, issues := e.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile cel: %w", issues.Err())
	}
	prg, err := e.celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program cel: %w", err)
	}
	return prg, nil
}
