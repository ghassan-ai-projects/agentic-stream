package app

import (
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func (s *session) holdUntilDrained(replies []map[string]any) bool {
	if s.held == nil && (!s.queue.pending() || !slices.ContainsFunc(replies, isBarrierAnswer)) {
		return false
	}
	s.held = append(s.held, replies...)
	return true
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

func (s *session) releaseHeld() string {
	if s.queue.pending() {
		return ""
	}
	held := s.held
	s.held = nil
	if err := s.send(held); err != nil {
		return endWriteFailed
	}
	return ""
}

func isBarrierAnswer(record map[string]any) bool {
	return record["type"] == domain.TypeHostSync
}
