package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const answerGrace = 2 * domain.HeartbeatInterval

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

func (h *Host) DispatchAuthorized(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (actionport.Effect, error) {
	order, live, err := h.prepare(ctx, command, authorization)
	if err != nil {
		return actionport.Effect{}, err
	}
	claim, err := h.admit(ctx, live, order)
	if err != nil {
		return actionport.Effect{}, err
	}
	effect, err := h.command(ctx, live, order)
	return h.settleAuthority(ctx, claim, effect, err)
}

func (h *Host) prepare(ctx context.Context, command actionport.Command, authorization actionport.Authorization) (domain.CommandOrder, *session, error) {
	order, err := h.orderFor(command)
	if err != nil {
		return domain.CommandOrder{}, nil, err
	}
	if err := authorize(ctx, authorization); err != nil {
		return domain.CommandOrder{}, nil, err
	}
	live := h.liveSession()
	order.ExpectedBootID, err = live.bootOf(ctx, order.Device)
	if err != nil {
		return domain.CommandOrder{}, nil, fmt.Errorf("plantwire: command %s not sent: %w", order.CommandID, err)
	}
	return order, live, nil
}

func (h *Host) VerifyDeviceCommand(context.Context, actionport.Command) (string, map[string]any, error) {
	return "", nil, nil
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
	deadline := h.clock.Now().Add(time.Duration(bounded.ExpiresAfterMs) * time.Millisecond)
	return domain.CommandOrder{CommandID: command.CommandID, IdempotencyKey: command.IdempotencyKey, PolicyDigest: command.PolicyDigest,
		Device: device, Command: bounded.Operation, Args: bounded.Parameters, DeadlineNS: deadline.UnixNano()}, nil
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

func (h *Host) command(ctx context.Context, live *session, order domain.CommandOrder) (actionport.Effect, error) {
	answers := make(chan domain.Settlement, mailboxSize)
	deadline := h.clock.NewTimer(time.Unix(0, order.DeadlineNS).Sub(h.clock.Now()) + answerGrace)
	defer deadline.Stop()
	if !live.submit(ctx, request{order: order, answers: answers}) {
		return actionport.Effect{}, fmt.Errorf("plantwire: command %s not sent: %w", order.CommandID, domain.ErrNoSession)
	}
	return await(ctx, order, answers, deadline)
}

func await(ctx context.Context, order domain.CommandOrder, answers <-chan domain.Settlement, deadline timer) (actionport.Effect, error) {
	var trail []map[string]any
	for {
		select {
		case settlement := <-answers:
			if trail = appendAnswer(trail, settlement); settlement.Terminal() {
				return effectOf(order, settlement, trail)
			}
		case <-deadline.C():
			return unknownEffect(order, trail, "no terminal answer before the command's deadline")
		case <-ctx.Done():
			return unknownEffect(order, trail, ctx.Err().Error())
		}
	}
}
