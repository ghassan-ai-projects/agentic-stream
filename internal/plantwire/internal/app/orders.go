package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const mailboxSize = 8

type request struct {
	order   domain.CommandOrder
	answers chan<- domain.Settlement
}

func (h *Host) goLive(s *session) {
	h.liveMu.Lock()
	defer h.liveMu.Unlock()
	h.live = s
}

func (h *Host) liveSession() *session {
	h.liveMu.Lock()
	defer h.liveMu.Unlock()
	return h.live
}

func (s *session) submit(ctx context.Context, req request) bool {
	select {
	case s.orders <- req:
		return true
	case <-s.done:
		return false
	case <-ctx.Done():
		return false
	}
}

func (s *session) order(req request) string {
	record, err := s.conversation.Command(req.order)
	if err != nil {
		req.answers <- domain.Settlement{CommandID: req.order.CommandID, Phase: domain.SettlementNotSent, Reason: err.Error(), Cause: err}
		return ""
	}
	s.mailboxes[req.order.CommandID] = req.answers
	if err := s.send([]map[string]any{record}); err != nil {
		return endWriteFailed
	}
	return ""
}

func (s *session) deliver(settlements []domain.Settlement) {
	for _, settlement := range settlements {
		mailbox := s.mailboxes[settlement.CommandID]
		if mailbox == nil {
			continue
		}
		select {
		case mailbox <- settlement:
		default:
		}
		if settlement.Terminal() {
			delete(s.mailboxes, settlement.CommandID)
		}
	}
}
