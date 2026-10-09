package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

// DueTimer is a pending processing-time timer whose due time has passed.
type DueTimer struct {
	ID, OperatorID, StateKey, DueAt, ExpectedEventID string
}

// ParseTimerPayload reads the event a timer expects to still be the last one.
func ParseTimerPayload(timerID string, payload []byte) (string, error) {
	var timerPayload struct {
		ExpectedEventID string `json:"expected_event_id"`
	}
	if err := json.Unmarshal(payload, &timerPayload); err != nil {
		return "", fmt.Errorf("decode timer payload %s: %w", timerID, err)
	}
	return timerPayload.ExpectedEventID, nil
}

// TimerFiring is a feature that fires one due timer.
type TimerFiring struct {
	Feature operators.Feature
	Timer   DueTimer
}

// InactiveTimerIDs marks as already matched every timer whose operator state is
// no longer active. Boot fencing intentionally suppresses stale timers so they
// cannot block the partition forever.
func InactiveTimerIDs(timers []DueTimer, stateActive func(stateKey string) bool) map[string]struct{} {
	matched := make(map[string]struct{}, len(timers))
	for _, timer := range timers {
		if !stateActive(timer.StateKey) {
			matched[timer.ID] = struct{}{}
		}
	}
	return matched
}

// MatchTimerFeatures pairs each feature with the due timer of its state key
// whose expected last event it carries, marking that timer matched.
func MatchTimerFeatures(timers []DueTimer, features []operators.Feature, matched map[string]struct{}) []TimerFiring {
	byState := indexTimers(timers)
	var firings []TimerFiring
	for _, feature := range features {
		timer, ok := byState[feature.OperatorID+"\x00"+timerStateKey(feature)]
		if !ok || !matchesExpectedEvent(feature, timer.ExpectedEventID) {
			continue
		}
		matched[timer.ID] = struct{}{}
		firings = append(firings, TimerFiring{Feature: feature, Timer: timer})
	}
	return firings
}

// RequireAllTimersMatched requires every due timer to have fired or belong to
// a fenced boot.
func RequireAllTimersMatched(matched map[string]struct{}, timers []DueTimer) error {
	if len(matched) != len(timers) {
		return fmt.Errorf("due timer has no matching operator state: matched %d of %d", len(matched), len(timers))
	}
	return nil
}

func indexTimers(timers []DueTimer) map[string]DueTimer {
	byState := make(map[string]DueTimer, len(timers))
	for _, timer := range timers {
		byState[timer.OperatorID+"\x00"+timer.StateKey] = timer
	}
	return byState
}

func timerStateKey(feature operators.Feature) string {
	if feature.StateKey != "" {
		return feature.StateKey
	}
	return feature.EntityID
}

func matchesExpectedEvent(feature operators.Feature, expectedEventID string) bool {
	if len(feature.InputEventIDs) == 0 {
		return false
	}
	return feature.InputEventIDs[len(feature.InputEventIDs)-1] == expectedEventID
}

// EnrichTimerFeature records the firing's provenance on the feature.
func EnrichTimerFeature(feature *operators.Feature, tenantID string, partitionID int, timer DueTimer, now time.Time, clockQuality string) {
	feature.TenantID = tenantID
	feature.PartitionID = partitionID
	feature.Metadata = map[string]any{
		"timer_id":               timer.ID,
		"timer_basis":            "processing_time",
		"timer_due_at":           timer.DueAt,
		"timer_fired_at":         kernel.FormatTime(now),
		"expected_event_horizon": timer.DueAt,
		"clock_quality":          clockQuality,
		"source_traceparent":     feature.Traceparent,
		"source_tracestate":      feature.Tracestate,
	}
}

type EntityRef struct{ Type, ID string }

func DistinctEntities(features []operators.Feature) []EntityRef {
	seen := make(map[EntityRef]struct{}, len(features))
	var entities []EntityRef
	for _, feature := range features {
		entity := EntityRef{Type: feature.EntityType, ID: feature.EntityID}
		if _, repeated := seen[entity]; !repeated {
			seen[entity] = struct{}{}
			entities = append(entities, entity)
		}
	}
	return entities
}
