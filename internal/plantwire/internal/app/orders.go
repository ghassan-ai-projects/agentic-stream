package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
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
	view.NodeNowNS = s.conversation.NodeNowNS(s.clock.Now())
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
	record, err := s.authorized(req)
	if err != nil {
		s.notSent(req, err)
		return ""
	}
	line, err := wire.Encode(record)
	if err != nil {
		s.conversation.Discard(record)
		s.notSent(req, err)
		return ""
	}
	s.conversation.Register(req.order)
	s.remember(req)
	return s.deliver(req, line)
}

func (s *session) authorized(req request) (map[string]any, error) {
	origin, err := s.conversation.OriginOf(req.order.Device)
	if err != nil {
		return nil, err
	}
	if err := s.settings.Authorizes(origin); err != nil {
		return nil, fmt.Errorf("%s: %w", domain.RefusalOriginNotAuthorized, err)
	}
	return s.conversation.Prepare(req.order)
}

func (s *session) notSent(req request, err error) {
	req.answers <- domain.Settlement{CommandID: req.order.CommandID, Phase: domain.SettlementNotSent, Reason: err.Error(), Cause: err}
	delete(s.mailboxes, req.order.CommandID)
	delete(s.watchers, req.order.CommandID)
}

func (s *session) remember(req request) {
	s.mailboxes[req.order.CommandID] = req.answers
	s.watchers[req.order.CommandID] = req.verified
}

func (s *session) deliver(req request, line []byte) string {
	written, err := s.conn.Write(line)
	if err == nil {
		return ""
	}
	if written == 0 {
		s.conversation.Forget(req.order.CommandID)
		s.notSent(req, err)
		return ""
	}
	return endWriteFailed
}

func (s *session) deliverReaction(reaction domain.Reaction) {
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
