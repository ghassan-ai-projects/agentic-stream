package domain

import "github.com/ghassan-ai-projects/agentic-stream/internal/spec"

type State struct {
	Open          map[RuleKey]bool
	FiresLastHour map[RuleKey]int
}

type Match struct {
	Rule       spec.Rule
	InEnvelope bool
	Features   map[string]any
}

type Routing struct {
	Fire       *Match
	RateCapped *Match
	Shadows    []Match
}

func (r *Rules) Route(features map[string]any, state State) Routing {
	var routing Routing
	var active []Match
	for _, compiled := range r.compiled {
		if state.Open[keyOf(compiled.rule)] || !compiled.matches(features) {
			continue
		}
		match := r.matchOf(compiled.rule, features)
		if compiled.rule.Mode == spec.RuleModeActive {
			active = append(active, match)
		} else {
			routing.Shadows = append(routing.Shadows, match)
		}
	}
	return r.answer(routing, active, state)
}

func (r *Rules) answer(routing Routing, active []Match, state State) Routing {
	if len(active) != 1 || !active[0].InEnvelope {
		return routing
	}
	match := active[0]
	if state.FiresLastHour[keyOf(match.Rule)] >= match.Rule.RateCapPerHour {
		routing.RateCapped = &match
		return routing
	}
	routing.Fire = &match
	return routing
}

func (r *Rules) matchOf(rule spec.Rule, features map[string]any) Match {
	read := readFeatures(append(rule.Reads, r.context...), features)
	return Match{Rule: rule, InEnvelope: insideEnvelope(rule.Envelope, features), Features: read}
}

func readFeatures(names []string, features map[string]any) map[string]any {
	read := make(map[string]any, len(names))
	for _, name := range names {
		if value, ok := features[name]; ok {
			read[name] = value
		}
	}
	return read
}

func insideEnvelope(envelope map[string]spec.Range, features map[string]any) bool {
	for name, bounds := range envelope {
		value, numeric := number(features[name])
		if !numeric || value < bounds.Min || value > bounds.Max {
			return false
		}
	}
	return true
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}
