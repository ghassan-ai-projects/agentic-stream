package app

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const reasoningPollInterval = 50 * time.Millisecond

func (s *session) holdUntilDrained(replies []map[string]any) bool {
	if s.held == nil && !s.shouldHold(replies) {
		return false
	}
	s.held = append(s.held, replies...)
	return true
}

func (s *session) shouldHold(replies []map[string]any) bool {
	return s.mayHoldBarriers() && slices.ContainsFunc(replies, isBarrierAnswer)
}

func (s *session) mayHoldBarriers() bool {
	return s.reasoning != nil || s.queue.backedUp()
}

func (s *session) pendingCalls() <-chan call {
	if s.held != nil {
		return nil
	}
	return s.calls
}

func (s *session) draining() <-chan struct{} {
	if s.held == nil {
		return nil
	}
	return s.queue.drained
}

func (s *session) rechecks() <-chan time.Time {
	if s.recheck == nil {
		return nil
	}
	return s.recheck.C()
}

func (s *session) releaseHeld(ctx context.Context) string {
	s.stopRecheck()
	if s.queue.pending() || s.reasoningInFlight(ctx) {
		return ""
	}
	held := s.held
	s.held = nil
	if err := s.send(held); err != nil {
		return endWriteFailed
	}
	return ""
}

func (s *session) reasoningInFlight(ctx context.Context) bool {
	if s.reasoning == nil {
		return false
	}
	busy, err := s.reasoning(ctx)
	if err != nil {
		slog.WarnContext(ctx, "plant wire host could not tell whether reasoning is in flight; answering the barrier", "error", err)
		return false
	}
	if busy {
		s.recheck = s.clock.NewTimer(reasoningPollInterval)
	}
	return busy
}

func (s *session) stopRecheck() {
	if s.recheck != nil {
		s.recheck.Stop()
		s.recheck = nil
	}
}

func isBarrierAnswer(record map[string]any) bool {
	return record["type"] == domain.TypeHostSync
}
