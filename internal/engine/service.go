package engine

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
)

// Service is the public facade over the stream engine use cases.
type Service struct{ app *app.Service }

// RunGlobal applies all partitions in durable event-log position order and
// reports how many events and timers it processed. Replay uses it so one
// virtual clock cannot observe a later partition before an earlier record in
// the authoritative trace.
func (s *Service) RunGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	return s.app.RunGlobal(ctx, beforeApply)
}

// SituationFeatures reads the CEL `features` view of a Situation from its
// current runtime state: the reduced facts as of the latest applied event, not
// as of its last published version. A Situation the engine does not hold
// reports found as false.
func (s *Service) SituationFeatures(ctx context.Context, situationID string) (map[string]any, bool, error) {
	return s.app.SituationFeatures(ctx, situationID)
}

// WakeSituations publishes a new version of each named open Situation from its
// current runtime state, marked as woken by a watch fire, and lets cognition
// evaluate its triggers on it (`delta.watch_fired`). A Situation the engine does
// not hold open is skipped.
func (s *Service) WakeSituations(ctx context.Context, situationIDs []string) error {
	return s.app.WakeSituations(ctx, situationIDs)
}

// FailedReasoning is a concluded episode that gave no accepted decision: the
// Situation and the version it reasoned on.
type FailedReasoning = domain.FailedReasoning

// RetryFailedReasoning wakes each open Situation whose failed reasoning is due
// for a retry under the spec's cognition.retryFailedReasoning: the Situation is
// still in the phase of its latest failure, at most Max failures happened in that
// phase, and After of event time passed since the latest failed version. The
// woken version carries `delta.reasoning_failed`. Without the policy it does
// nothing.
func (s *Service) RetryFailedReasoning(ctx context.Context, failed []FailedReasoning) error {
	return s.app.RetryFailedReasoning(ctx, failed)
}
