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

const AnswerGrace = 2 * domain.HeartbeatInterval

var (
	ErrAuthorizationRequired = errors.New("plantwire: a command needs the final authorization check")
	ErrDeviceNotInSite       = errors.New("plantwire: the site file names no device for the command's target")
	ErrCommandRefused        = errors.New("plantwire: the command was refused")
	ErrCommandFailed         = errors.New("plantwire: the device reported the command failed")
)

func (h *Host) OwnsRoute(route string) bool {
	return route != actionport.WatchInstallRoute && h.catalog.NamesRoute(route)
}

func (h *Host) Dispatch(context.Context, actionport.Command) (actionport.Effect, error) {
	return actionport.Effect{}, ErrAuthorizationRequired
}

type target struct {
	order    domain.CommandOrder
	named    string
	live     *session
	view     domain.DeviceView
	verified chan domain.Verification
	window   timer
	answerBy time.Time
}

func (h *Host) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	t, err := h.prepare(ctx, command, authorization)
	if err != nil {
		return actionport.Effect{}, err
	}
	if refused, err := h.checkOrigin(t); err != nil {
		return refused, err
	}
	claim, err := h.admit(ctx, t)
	if err != nil {
		return actionport.Effect{}, err
	}
	effect, err := h.command(ctx, t, claim)
	effect, err = h.settleAuthority(ctx, claim, effect, err)
	h.expectVerification(t, effect, err)
	return effect, err
}

func (h *Host) prepare(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (target, error) {
	order, err := h.orderFor(command)
	if err != nil {
		return target{}, err
	}
	live := h.liveSession()
	view, err := live.deviceOf(ctx, order.Device)
	if err != nil {
		return target{}, fmt.Errorf("plantwire: command %s not sent: %w", order.CommandID, err)
	}
	if err := authorize(ctx, authorization); err != nil {
		return target{}, err
	}
	order.ExpectedBootID, order.DeadlineNS = view.BootID, view.NodeNowNS+order.Expiry.Nanoseconds()
	return target{order: order, named: command.NormalizedTarget, live: live, view: view, verified: make(chan domain.Verification, 1),
		answerBy: h.clock.Now().Add(order.Expiry)}, nil
}

func (h *Host) checkOrigin(t target) (actionport.Effect, error) {
	if err := h.settings.Authorizes(t.view.Origin); err != nil {
		return originRefused(t.order, t.view.Origin), fmt.Errorf("plantwire: command %s not sent: %w", t.order.CommandID, err)
	}
	return actionport.Effect{}, nil
}

func (h *Host) orderFor(command actionport.Command) (domain.CommandOrder, error) {
	bounded, err := h.catalog.Bound(command)
	if err != nil {
		return domain.CommandOrder{}, fmt.Errorf("plantwire: command %s: %w", command.CommandID, err)
	}
	device, named := h.mapping.Directory().DeviceFor(bounded.Target)
	if !named {
		return domain.CommandOrder{}, fmt.Errorf("%w: command %s for %s", ErrDeviceNotInSite, command.CommandID, bounded.Target)
	}
	return domain.CommandOrder{CommandID: command.CommandID, IdempotencyKey: command.IdempotencyKey, PolicyDigest: command.PolicyDigest,
		Device: device, Command: bounded.Operation, Args: bounded.Parameters,
		Expiry: time.Duration(bounded.ExpiresAfterMs) * time.Millisecond}, nil
}

func authorize(ctx context.Context, authorization actionport.Authorization) error {
	if authorization.Check == nil {
		return ErrAuthorizationRequired
	}
	if err := authorization.Check(ctx); err != nil {
		return fmt.Errorf("plantwire: final authorization: %w", err)
	}
	return nil
}

func (h *Host) command(ctx context.Context, t target, claim authority.TargetClaim) (actionport.Effect, error) {
	answers := make(chan domain.Settlement, mailboxSize)
	deadline := h.clock.NewTimer(t.answerBy.Sub(h.clock.Now()) + AnswerGrace)
	defer deadline.Stop()
	wait := &waiting{host: h, live: t.live, order: t.order, answers: answers, deadline: deadline, claim: claim}
	wait.startRenewal()
	defer wait.stopRenewal()
	if !t.live.submit(ctx, request{order: t.order, answers: answers, verified: t.verified}) {
		return actionport.Effect{}, fmt.Errorf("plantwire: command %s not sent: %w", t.order.CommandID, domain.ErrNoSession)
	}
	return wait.answer(ctx)
}
