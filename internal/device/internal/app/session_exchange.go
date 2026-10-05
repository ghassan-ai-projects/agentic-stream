package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

// Exchange is the ordered terminal response to one device command.
// Receipt proves admission; Result is the device-reported terminal execution
// status. Neither is physical confirmation by itself.
type Exchange struct {
	Receipt map[string]any
	Result  map[string]any
}

// Exchange sends one already-materialized command and consumes its
// receipt and terminal result. The bool reports whether bytes were handed to
// the transport; callers must treat a post-send receive error as an unknown
// outcome.
func (s *Session) Exchange(ctx context.Context, command map[string]any) (Exchange, bool, error) {
	exchange, sent, err := s.exchange(ctx, command)
	if exchange == nil {
		return Exchange{}, sent, err
	}
	return *exchange, sent, err
}

func (s *Session) exchange(ctx context.Context, command map[string]any) (*Exchange, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nil, false, err
	}
	if s.isSafeStopLatched() {
		return nil, false, fmt.Errorf("safe stop has priority over ordinary device commands")
	}
	return s.exchangeOrdinaryCommand(ctx, command)
}

func (s *Session) ensureOpen() error {
	if !s.opened || s.closed {
		return fmt.Errorf("device session is not open")
	}
	return nil
}

func (s *Session) exchangeOrdinaryCommand(ctx context.Context, command map[string]any) (*Exchange, bool, error) {
	frame, commandIdentity, idempotencyKey, err := prepareCommand(command)
	if err != nil {
		return nil, false, err
	}
	if err := s.validateCommandLifetime(command); err != nil {
		return nil, false, err
	}
	if s.reconciliationRequired {
		return nil, false, deviceauthority.ErrReconciliationRequired
	}
	claim, err := s.claimCommand(ctx, command, commandIdentity)
	if err != nil {
		return nil, false, err
	}
	return s.deliverClaimedCommand(ctx, command, claim, frame, commandIdentity, idempotencyKey)
}

func prepareCommand(command map[string]any) ([]byte, string, string, error) {
	frame, err := wire.Encode(command)
	if err != nil {
		return nil, "", "", fmt.Errorf("encode device command: %w", err)
	}
	commandIdentity, err := domain.CommandIdentity(command)
	if err != nil {
		return nil, "", "", fmt.Errorf("digest device command identity: %w", err)
	}
	idempotencyKey, _ := command["idempotency_key"].(string)
	return frame, commandIdentity, idempotencyKey, nil
}

func (s *Session) validateCommandLifetime(command map[string]any) error {
	expectedBoot, _ := command["expected_boot_id"].(string)
	if expectedBoot != s.bootID {
		return fmt.Errorf("device boot changed: command expects %q, session is %q", expectedBoot, s.bootID)
	}
	return nil
}

func (s *Session) claimCommand(ctx context.Context, command map[string]any, commandIdentity string) (deviceauthority.TargetClaim, error) {
	target, _ := command["target"].(string)
	claim := s.targetClaim(target)
	if err := s.authority.Claim(ctx, claim); err != nil {
		return claim, fmt.Errorf("claim device target: %w", err)
	}
	return s.bindClaimedCommand(ctx, command, claim, commandIdentity)
}

func (s *Session) bindClaimedCommand(ctx context.Context, command map[string]any, claim deviceauthority.TargetClaim, commandIdentity string) (deviceauthority.TargetClaim, error) {
	if err := s.authority.BindCommand(ctx, deviceauthority.CommandBinding{
		CommandID: documentString(command, "command_id"), Target: claim.Target, Device: claim.Device,
		Owner: claim.Owner, CommandDigest: commandIdentity,
	}); err != nil {
		releaseErr := s.authority.ReleaseClaim(ctx, claim)
		if errors.Is(releaseErr, deviceauthority.ErrTargetClaimNotOwned) {
			releaseErr = nil
		}
		return claim, fmt.Errorf("bind device command: %w", errors.Join(err, releaseErr))
	}
	return s.assertClaimBeforeDelivery(ctx, claim)
}

func (s *Session) assertClaimBeforeDelivery(ctx context.Context, claim deviceauthority.TargetClaim) (deviceauthority.TargetClaim, error) {
	s.claimedTargets[claim.Target] = struct{}{}
	// Claim performs the durable admission check. Repeat it directly before
	// transport delivery to minimize the revoke-to-send race; a post-send
	// failure remains an unknown outcome because bytes cannot be retracted.
	if err := s.authority.AssertClaim(ctx, claim); err != nil {
		return claim, fmt.Errorf("assert device target authority: %w", err)
	}
	return claim, nil
}

func (s *Session) deliverClaimedCommand(ctx context.Context, command map[string]any, claim deviceauthority.TargetClaim, frame []byte, commandIdentity, idempotencyKey string) (*Exchange, bool, error) {
	if receipt, ok, err := s.cachedReceipt(idempotencyKey, commandIdentity); ok || err != nil {
		if err != nil {
			return nil, ok, err
		}
		return &Exchange{Receipt: receipt, Result: cloneDocument(s.receipts[idempotencyKey].result)}, ok, nil
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		sent := transport.MayHaveSent(err)
		if sent {
			return s.unknownDeviceOutcome(ctx, nil, errors.Join(err, fmt.Errorf("command send may have crossed the gateway")))
		}
		return nil, sent, fmt.Errorf("send device command: %w", err)
	}
	return s.receiveCommandOutcome(ctx, command, claim, commandIdentity, idempotencyKey)
}

func (s *Session) cachedReceipt(idempotencyKey, commandIdentity string) (map[string]any, bool, error) {
	cached, ok := s.receipts[idempotencyKey]
	if !ok {
		return nil, false, nil
	}
	if cached.commandDigest != commandIdentity {
		return nil, false, fmt.Errorf("idempotency key %q conflicts with the prior device command", idempotencyKey)
	}
	return cloneDocument(cached.receipt), true, nil
}

func (s *Session) receiveCommandOutcome(ctx context.Context, command map[string]any, claim deviceauthority.TargetClaim, commandIdentity, idempotencyKey string) (*Exchange, bool, error) {
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		return s.unknownDeviceOutcome(ctx, nil, err)
	}
	receipt, err := wire.Decode(reply)
	if err != nil {
		s.telemetry.ObserveDeviceFrameError()
		return s.unknownDeviceOutcome(ctx, nil, fmt.Errorf("decode device receipt: %w", err))
	}
	if !domain.ReceiptMatches(receipt, command, s.bootID) {
		return s.unknownDeviceOutcome(ctx, nil, errors.New("device receipt identity mismatch"))
	}
	return s.completeCommandOutcome(ctx, command, receipt, claim, commandIdentity, idempotencyKey)
}

func (s *Session) completeCommandOutcome(ctx context.Context, command, receipt map[string]any, claim deviceauthority.TargetClaim, commandIdentity, idempotencyKey string) (*Exchange, bool, error) {
	partial := &Exchange{Receipt: receipt}
	if err := s.authority.AssertClaim(ctx, claim); err != nil {
		return s.unknownDeviceOutcome(ctx, partial, fmt.Errorf("authority lost during device exchange: %w", err))
	}
	result, err := s.receiveDeviceResult(ctx, command, receipt)
	if err != nil {
		return s.unknownDeviceOutcome(ctx, partial, err)
	}
	return s.cacheCommandOutcome(ctx, partial, result, claim, commandIdentity, idempotencyKey)
}

func (s *Session) receiveDeviceResult(ctx context.Context, command, receipt map[string]any) (map[string]any, error) {
	resultFrame, err := s.transport.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("receive device result: %w", err)
	}
	result, err := wire.Decode(resultFrame)
	if err != nil {
		s.telemetry.ObserveDeviceFrameError()
		return nil, fmt.Errorf("decode device result: %w", err)
	}
	if !domain.ResultMatches(result, command, s.bootID, receipt) {
		return nil, errors.New("device result identity or status mismatch")
	}
	return result, nil
}

func (s *Session) cacheCommandOutcome(ctx context.Context, partial *Exchange, result map[string]any, claim deviceauthority.TargetClaim, commandIdentity, idempotencyKey string) (*Exchange, bool, error) {
	partial.Result = result
	if err := s.authority.AssertClaim(ctx, claim); err != nil {
		return s.unknownDeviceOutcome(ctx, partial, fmt.Errorf("authority lost during device result: %w", err))
	}
	s.receipts[idempotencyKey] = cachedReceipt{commandDigest: commandIdentity, receipt: cloneDocument(partial.Receipt), result: cloneDocument(result)}
	return &Exchange{Receipt: partial.Receipt, Result: result}, true, nil
}

func (s *Session) unknownDeviceOutcome(ctx context.Context, partial *Exchange, err error) (*Exchange, bool, error) {
	barrierErr := s.requireReconciliation(ctx, "device exchange was not trustworthy")
	// A malformed, incomplete, or mismatched pair leaves the next frame's
	// meaning unknowable. Do not let a caller reuse a potentially desynchronized
	// transport; a fresh handshake is required.
	s.invalidateTransportLocked()
	return partial, true, &deviceExchangeError{err: errors.Join(err, barrierErr)}
}
