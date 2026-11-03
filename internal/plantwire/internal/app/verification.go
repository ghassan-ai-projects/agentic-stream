package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

const VerificationWindow = 6 * time.Second

func (h *Host) VerifyDeviceCommand(ctx context.Context, command actionport.Command) (string, map[string]any, error) {
	t, sent := h.takeSent(command.CommandID)
	if !sent {
		return "", nil, nil
	}
	if t.unknownWithoutReadback() {
		t.window.Stop()
		return unreadBy(t.order)
	}
	verification := t.awaitReadings(ctx)
	if !verification.Verified {
		slog.InfoContext(ctx, "plant wire command not verified: no matching reading of its outputs", "command_id", t.order.CommandID,
			"device", t.order.Device, "window", VerificationWindow)
		return "", nil, nil
	}
	return verifiedBy(t.order, verification)
}

func (h *Host) expectVerification(t target, effect actionport.Effect, err error) {
	if !effect.VerificationPending && !actionport.IsUnknownOutcome(err) {
		return
	}
	t.window = h.clock.NewTimer(VerificationWindow)
	t.unknown = actionport.IsUnknownOutcome(err)
	h.sentMu.Lock()
	defer h.sentMu.Unlock()
	h.sent[t.order.CommandID] = t
}

func (h *Host) takeSent(commandID string) (target, bool) {
	h.sentMu.Lock()
	defer h.sentMu.Unlock()
	t, sent := h.sent[commandID]
	delete(h.sent, commandID)
	return t, sent
}

func (t target) unknownWithoutReadback() bool {
	return t.unknown && !t.view.ReadsBack(t.order.Args)
}

func (t target) awaitReadings(ctx context.Context) domain.Verification {
	defer t.window.Stop()
	select {
	case verification := <-t.verified:
		return verification
	case <-t.window.C():
	case <-ctx.Done():
	case <-t.live.done:
	}
	return t.live.unwatch(ctx, t.order.CommandID, t.verified)
}

func verifiedBy(order domain.CommandOrder, verification domain.Verification) (string, map[string]any, error) {
	return sealedAs(actionport.CommandSucceeded, order, verification)
}

func unreadBy(order domain.CommandOrder) (string, map[string]any, error) {
	return sealedAs(actionport.CommandManualReview, order, domain.Verification{CommandID: order.CommandID, Device: order.Device, BootID: order.ExpectedBootID})
}

func sealedAs(status string, order domain.CommandOrder, verification domain.Verification) (string, map[string]any, error) {
	device := authority.DeviceBoot{DeviceID: order.Device, BootID: order.ExpectedBootID}
	sealed, err := authority.SealReconciliationEvidence(readingSource, order.Device, deviceState(device), verification.Feedback())
	if err != nil {
		return "", nil, fmt.Errorf("plantwire: seal the readings of %s: %w", order.CommandID, err)
	}
	return status, sealed.Document(), nil
}

func (s *session) unwatch(ctx context.Context, commandID string, verified <-chan domain.Verification) domain.Verification {
	s.call(context.WithoutCancel(ctx), func() string {
		s.conversation.Unwatch(commandID)
		delete(s.watchers, commandID)
		return ""
	})
	select {
	case verification := <-verified:
		return verification
	default:
		return domain.Verification{CommandID: commandID}
	}
}

func (s *session) verified(verifications []domain.Verification) {
	for _, verification := range verifications {
		if watcher := s.watchers[verification.CommandID]; watcher != nil {
			watcher <- verification
			delete(s.watchers, verification.CommandID)
		}
	}
}
