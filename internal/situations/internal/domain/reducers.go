package domain

import (
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

func (e *Engine) applyFeatureEvidence(sit *Situation, feature operators.Feature) bool {
	completenessChanged := feature.Completeness != "" && feature.Completeness != sit.Completeness
	if feature.Completeness != "" {
		sit.Completeness = feature.Completeness
	}
	e.applyReducers(sit, feature)
	recordTimerProvenance(sit, feature.Metadata)
	if feature.EventTime.After(sit.LatestEventTime) {
		sit.LatestEventTime = feature.EventTime
	}
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
			addEvidence(sit, feature.InputEventIDs, evidenceLimit(r.Limit))
		}
	}
}

func keepLatestFact(sit *Situation, field string, feature operators.Feature) {
	_, exists := sit.Facts[field]
	currentTime, _ := sit.Facts[field+"_event_time"].(time.Time)
	if !exists || feature.EventTime.After(currentTime) {
		sit.Facts[field] = feature.Value
		sit.Facts[field+"_event_time"] = feature.EventTime
	}
}

const MaxEvidence = 4096

func evidenceLimit(declared int) int {
	if declared <= 0 || declared > MaxEvidence {
		return MaxEvidence
	}
	return declared
}

func addEvidence(sit *Situation, eventIDs []string, limit int) {
	batch := newEvidenceBatch(eventIDs)
	sit.Evidence = append(slices.DeleteFunc(sit.Evidence, batch.contains), batch.order...)
	sit.Evidence = keepNewest(sit.Evidence, limit)
}

type evidenceBatch struct {
	order   []string
	members map[string]struct{}
}

func newEvidenceBatch(eventIDs []string) evidenceBatch {
	batch := evidenceBatch{members: make(map[string]struct{}, len(eventIDs))}
	for _, id := range eventIDs {
		if !batch.contains(id) {
			batch.members[id] = struct{}{}
			batch.order = append(batch.order, id)
		}
	}
	return batch
}

func (b evidenceBatch) contains(id string) bool {
	_, member := b.members[id]
	return member
}

func keepNewest(evidence []string, limit int) []string {
	if excess := len(evidence) - limit; excess > 0 {
		return slices.Delete(evidence, 0, excess)
	}
	return evidence
}
