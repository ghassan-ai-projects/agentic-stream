package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const mailboxSize = 8

type request struct {
	order   domain.CommandOrder
	answers chan<- domain.Settlement
}

type lookup struct {
	device string
	reply  chan<- bootReply
}

type bootReply struct {
	boot string
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

func (s *session) bootOf(ctx context.Context, device string) (string, error) {
	if s == nil {
		return "", domain.ErrNoSession
	}
	reply := make(chan bootReply, 1)
	select {
	case s.lookups <- lookup{device: device, reply: reply}:
	case <-s.done:
		return "", domain.ErrNoSession
	case <-ctx.Done():
		return "", fmt.Errorf("plantwire: look up the boot of %s: %w", device, ctx.Err())
	}
	answer := <-reply
	return answer.boot, answer.err
}

func (s *session) answerLookup(l lookup) string {
	boot, err := s.conversation.BootOf(l.device)
	l.reply <- bootReply{boot: boot, err: err}
	return ""
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
