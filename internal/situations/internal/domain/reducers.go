package domain

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

// applyFeatureEvidence folds one feature into the Situation's facts, evidence
// and trace context, reporting whether its completeness changed.
func (e *Engine) applyFeatureEvidence(sit *Situation, feature operators.Feature) bool {
	completenessChanged := feature.Completeness != "" && feature.Completeness != sit.Completeness
	if feature.Completeness != "" {
		sit.Completeness = feature.Completeness
	}
	e.applyReducers(sit, feature)
	recordTimerProvenance(sit, feature.Metadata)
	sit.LatestEventTime = feature.EventTime
	if feature.Traceparent != "" || !feature.TraceContinuation {
		sit.Traceparent = feature.Traceparent
		sit.Tracestate = feature.Tracestate
	}
	return completenessChanged
}

func recordTimerProvenance(sit *Situation, metadata map[string]any) {
	if len(metadata) == 0 {
		return
	}
	if sit.Facts == nil {
		sit.Facts = make(map[string]any)
	}
	sit.Facts["timer_provenance"] = cloneMap(metadata)
}

func (e *Engine) applyReducers(sit *Situation, feature operators.Feature) {
	for _, r := range e.spec.Situation.Reducers {
		if r.Input != feature.OutputName {
			continue
		}
		switch r.Strategy {
		case "latest_event_time":
			keepLatestFact(sit, r.Field, feature)
		case "set_union":
			addEvidence(sit, feature.InputEventIDs)
		}
	}
}

// keepLatestFact stores the feature value unless the fact already holds a
// value from a later or equal event time.
func keepLatestFact(sit *Situation, field string, feature operators.Feature) {
	_, exists := sit.Facts[field]
	currentTime, _ := sit.Facts[field+"_event_time"].(time.Time)
	if !exists || feature.EventTime.After(currentTime) {
		sit.Facts[field] = feature.Value
		sit.Facts[field+"_event_time"] = feature.EventTime
	}
}

func addEvidence(sit *Situation, eventIDs []string) {
	if sit.Evidence == nil {
		sit.Evidence = make(map[string]struct{})
	}
	for _, id := range eventIDs {
		sit.Evidence[id] = struct{}{}
	}
}
