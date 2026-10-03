package engine

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
)

// applyMatchedTimerFeatures applies each feature that fires a due timer for
// its expected last event. Every timer must either fire or belong to a
// fenced (inactive) boot.
func (e *Engine) applyMatchedTimerFeatures(ctx context.Context, tx *sql.Tx, partitionID int, state *operators.PartitionState, timers []dueTimer, features []operators.Feature, watermark, now time.Time) ([]operators.Feature, error) {
	matched := activeTimerIDs(e.opRuntime, state, timers)
	firings := matchTimerFeatures(indexTimers(timers), features, matched)
	var applied []operators.Feature
	for _, firing := range firings {
		feature, err := e.fireTimer(ctx, tx, partitionID, firing, watermark, now)
		if err != nil {
			return nil, err
		}
		applied = append(applied, feature)
	}
	if len(matched) != len(timers) {
		return nil, fmt.Errorf("due timer has no matching operator state: matched %d of %d", len(matched), len(timers))
	}
	return applied, nil
}

// timerFiring is a feature that fires one due timer.
type timerFiring struct {
	feature operators.Feature
	timer   dueTimer
}

// matchTimerFeatures pairs each feature with the due timer of its state key
// whose expected last event it carries, marking that timer matched.
func matchTimerFeatures(timersByState map[string]dueTimer, features []operators.Feature, matched map[string]struct{}) []timerFiring {
	var firings []timerFiring
	for _, feature := range features {
		timer, ok := timersByState[feature.OperatorID+"\x00"+timerStateKey(feature)]
		if !ok || !matchesExpectedEvent(feature, timer.expectedEventID) {
			continue
		}
		matched[timer.id] = struct{}{}
		firings = append(firings, timerFiring{feature: feature, timer: timer})
	}
	return firings
}

// fireTimer records the firing's provenance and applies the feature.
func (e *Engine) fireTimer(ctx context.Context, tx *sql.Tx, partitionID int, firing timerFiring, watermark, now time.Time) (operators.Feature, error) {
	feature := firing.feature
	enrichTimerFeature(&feature, e.tenantID, partitionID, firing.timer, now, e.clock)
	return feature, e.saveTimerFeature(ctx, tx, partitionID, feature, watermark)
}

func indexTimers(timers []dueTimer) map[string]dueTimer {
	byState := make(map[string]dueTimer, len(timers))
	for _, timer := range timers {
		byState[timer.operatorID+"\x00"+timer.stateKey] = timer
	}
	return byState
}

func activeTimerIDs(runtime *operators.OperatorRuntime, state *operators.PartitionState, timers []dueTimer) map[string]struct{} {
	matched := make(map[string]struct{}, len(timers))
	for _, timer := range timers {
		if !runtime.IsTimerStateActive(state, timer.stateKey) {
			// Boot fencing intentionally suppresses stale timers so they cannot
			// block the partition forever.
			matched[timer.id] = struct{}{}
		}
	}
	return matched
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

func enrichTimerFeature(feature *operators.Feature, tenantID string, partitionID int, timer dueTimer, now time.Time, clk clock.Clock) {
	feature.TenantID = tenantID
	feature.PartitionID = partitionID
	feature.Metadata = map[string]any{
		"timer_id":               timer.id,
		"timer_basis":            "processing_time",
		"timer_due_at":           timer.dueAt,
		"timer_fired_at":         now.Format(time.RFC3339Nano),
		"expected_event_horizon": timer.dueAt,
		"clock_quality":          clock.Quality(clk),
		"source_traceparent":     feature.Traceparent,
		"source_tracestate":      feature.Tracestate,
	}
}

func (e *Engine) saveTimerFeature(ctx context.Context, tx *sql.Tx, partitionID int, feature operators.Feature, watermark time.Time) error {
	versions, err := e.sitEngine.ApplyFeature(ctx, feature, watermark)
	if err != nil {
		return fmt.Errorf("apply timer situation: %w", err)
	}
	for _, version := range versions {
		if err := e.saveSituationVersion(ctx, tx, partitionID, version); err != nil {
			return fmt.Errorf("save timer situation version: %w", err)
		}
		if e.cogEngine != nil {
			if err := e.cogEngine.Process(ctx, tx, version); err != nil {
				return fmt.Errorf("process timer cognition: %w", err)
			}
		}
	}
	return nil
}
