package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const mailboxSize = 8

type call func() string

type request struct {
	order    domain.CommandOrder
	answers  chan<- domain.Settlement
	verified chan<- domain.Verification
}

type deviceReply struct {
	view domain.DeviceView
	err  error
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

func (s *session) call(ctx context.Context, do call) bool {
	select {
	case s.calls <- do:
		return true
	case <-s.done:
		return false
	case <-ctx.Done():
		return false
	}
}

func (s *session) deviceOf(ctx context.Context, device string) (domain.DeviceView, error) {
	if s == nil {
		return domain.DeviceView{}, domain.ErrNoSession
	}
	reply := make(chan deviceReply, 1)
	if !s.call(ctx, func() string { reply <- s.lookUp(device); return "" }) {
		return domain.DeviceView{}, unavailable(ctx, device)
	}
	answer := <-reply
	return answer.view, answer.err
}

func (s *session) lookUp(device string) deviceReply {
	view, err := s.conversation.Device(device)
	return deviceReply{view: view, err: err}
}

func unavailable(ctx context.Context, device string) error {
	if ctx.Err() != nil {
		return fmt.Errorf("plantwire: look up %s: %w", device, ctx.Err())
	}
	return domain.ErrNoSession
}

func (s *session) submit(ctx context.Context, req request) bool {
	return s.call(ctx, func() string { return s.order(req) })
}

func (s *session) order(req request) string {
	record, err := s.conversation.Command(req.order)
	if err != nil {
		req.answers <- domain.Settlement{CommandID: req.order.CommandID, Phase: domain.SettlementNotSent, Reason: err.Error(), Cause: err}
		return ""
	}
	s.mailboxes[req.order.CommandID] = req.answers
	s.watchers[req.order.CommandID] = req.verified
	if err := s.send([]map[string]any{record}); err != nil {
		return endWriteFailed
	}
	return ""
}

func (s *session) deliver(reaction domain.Reaction) {
	s.settle(reaction.Settlements)
	s.verified(reaction.Verifications)
}

func (s *session) settle(settlements []domain.Settlement) {
	for _, settlement := range settlements {
		if mailbox := s.mailboxes[settlement.CommandID]; mailbox != nil {
			select {
			case mailbox <- settlement:
			default:
			}
		}
		s.forget(settlement)
	}
}

func (s *session) forget(settlement domain.Settlement) {
	if !settlement.Terminal() {
		return
	}
	delete(s.mailboxes, settlement.CommandID)
	if !settlement.Verifiable() {
		delete(s.watchers, settlement.CommandID)
	}
}
