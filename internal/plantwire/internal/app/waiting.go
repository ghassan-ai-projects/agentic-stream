package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

var errNoAnswer = errors.New("no terminal answer before the command's deadline")

type waiting struct {
	host     *Host
	live     *session
	order    domain.CommandOrder
	answers  <-chan domain.Settlement
	deadline timer
	claim    authority.TargetClaim
	tick     timer
}

func (w *waiting) startRenewal() {
	lease := w.host.authority.ClaimLease()
	if lease <= 0 {
		return
	}
	w.tick = w.host.clock.NewTimer(lease / 2)
}

func (w *waiting) stopRenewal() {
	if w.tick != nil {
		w.tick.Stop()
	}
}

func (w *waiting) tickChannel() <-chan time.Time {
	if w.tick == nil {
		return nil
	}
	return w.tick.C()
}

func (w *waiting) renewClaim(ctx context.Context) error {
	if w.live.ended() {
		return nil
	}
	if err := w.host.authority.Claim(ctx, w.claim); err != nil {
		return fmt.Errorf("plantwire: the target claim could not be renewed: %w", err)
	}
	w.rearm()
	return nil
}

func (w *waiting) rearm() {
	if w.tick == nil {
		return
	}
	w.tick.Stop()
	w.tick = w.host.clock.NewTimer(w.host.authority.ClaimLease() / 2)
}

func (w *waiting) answer(ctx context.Context) (actionport.Effect, error) {
	var trail []map[string]any
	for {
		settlement, given, err := w.next(ctx)
		if err != nil {
			return unknownEffect(w.order, trail, err.Error())
		}
		if !given {
			continue
		}
		if trail = appendAnswer(trail, settlement); settlement.Terminal() {
			return effectOf(w.order, settlement, trail)
		}
	}
}

func (w *waiting) next(ctx context.Context) (domain.Settlement, bool, error) {
	select {
	case settlement := <-w.answers:
		return settlement, true, w.renewClaim(ctx)
	case <-w.tickChannel():
		return domain.Settlement{}, false, w.renewClaim(ctx)
	case <-w.deadline.C():
		return domain.Settlement{}, false, errNoAnswer
	case <-ctx.Done():
		return domain.Settlement{}, false, fmt.Errorf("plantwire: the command's context ended: %w", ctx.Err())
	}
}
