package app

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

// Exchange is the ordered terminal response to one device command.
// Receipt proves admission; Result is the device-reported terminal execution
// status. Neither is physical confirmation by itself.
type Exchange struct {
	Receipt domain.Receipt
	Result  domain.Result
}

// clone copies the records' documents, so a caller cannot change what the
// session cached.
func (e Exchange) clone() Exchange {
	e.Receipt.Document = maps.Clone(e.Receipt.Document)
	e.Result.Document = maps.Clone(e.Result.Document)
	e.Receipt.RejectCode = cloneCode(e.Receipt.RejectCode)
	e.Result.ErrorCode = cloneCode(e.Result.ErrorCode)
	return e
}

func cloneCode(code *string) *string {
	if code == nil {
		return nil
	}
	value := *code
	return &value
}

// document preserves the provider result contract, including absent replies.
func (e Exchange) document() map[string]any {
	return map[string]any{"receipt": e.Receipt.Document, "result": e.Result.Document}
}

// Exchange sends one already-materialized command and consumes its
// receipt and terminal result. The bool reports whether bytes were handed to
// the transport; callers must treat a post-send receive error as an unknown
// outcome.
func (s *Session) Exchange(ctx context.Context, command domain.Command) (Exchange, bool, error) {
	exchange, sent, err := s.exchange(ctx, command)
	if exchange == nil {
		return Exchange{}, sent, err
	}
	return *exchange, sent, err
}

func (s *Session) exchange(ctx context.Context, command domain.Command) (*Exchange, bool, error) {
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

func (s *Session) exchangeOrdinaryCommand(ctx context.Context, command domain.Command) (*Exchange, bool, error) {
	frame, commandIdentity, err := prepareCommand(command)
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
	return s.deliverClaimedCommand(ctx, command, claim, frame, commandIdentity)
}

func prepareCommand(command domain.Command) ([]byte, string, error) {
	frame, err := wire.Encode(command.Document())
	if err != nil {
		return nil, "", fmt.Errorf("encode device command: %w", err)
	}
	commandIdentity, err := command.Identity()
	if err != nil {
		return nil, "", err
	}
	return frame, commandIdentity, nil
}

func (s *Session) validateCommandLifetime(command domain.Command) error {
	if command.ExpectedBootID != s.bootID {
		return fmt.Errorf("device boot changed: command expects %q, session is %q", command.ExpectedBootID, s.bootID)
	}
	return nil
}

func (s *Session) claimCommand(ctx context.Context, command domain.Command, commandIdentity string) (deviceauthority.TargetClaim, error) {
	claim := s.targetClaim(command.Target)
	if err := s.authority.Claim(ctx, claim); err != nil {
		return claim, fmt.Errorf("claim device target: %w", err)
	}
	return s.bindClaimedCommand(ctx, command, claim, commandIdentity)
}

func (s *Session) bindClaimedCommand(ctx context.Context, command domain.Command, claim deviceauthority.TargetClaim, commandIdentity string) (deviceauthority.TargetClaim, error) {
	if err := s.authority.BindCommand(ctx, deviceauthority.CommandBinding{
		CommandID: command.CommandID, Target: claim.Target, Device: claim.Device,
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

func (s *Session) deliverClaimedCommand(ctx context.Context, command domain.Command, claim deviceauthority.TargetClaim, frame []byte, commandIdentity string) (*Exchange, bool, error) {
	if cached, ok, err := s.cachedExchange(command.IdempotencyKey, commandIdentity); ok || err != nil {
		return cached, ok, err
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		sent := transport.MayHaveSent(err)
		if sent {
			return s.unknownDeviceOutcome(ctx, nil, errors.Join(err, fmt.Errorf("command send may have crossed the gateway")))
		}
		return nil, sent, fmt.Errorf("send device command: %w", err)
	}
	return s.receiveCommandOutcome(ctx, command, claim, commandIdentity)
}

// cachedExchange returns the first answer to a repeated idempotency key. A key
// reused for a different command is a conflict.
func (s *Session) cachedExchange(idempotencyKey, commandIdentity string) (*Exchange, bool, error) {
	cached, ok := s.receipts[idempotencyKey]
	if !ok {
		return nil, false, nil
	}
	if cached.commandIdentity != commandIdentity {
		return nil, false, fmt.Errorf("idempotency key %q conflicts with the prior device command", idempotencyKey)
	}
	exchange := cached.exchange.clone()
	return &exchange, true, nil
}

func (s *Session) receiveCommandOutcome(ctx context.Context, command domain.Command, claim deviceauthority.TargetClaim, commandIdentity string) (*Exchange, bool, error) {
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		return s.unknownDeviceOutcome(ctx, nil, err)
	}
	receipt, err := wire.DecodeReceipt(reply)
	if err != nil {
		s.telemetry.ObserveDeviceFrameError()
		return s.unknownDeviceOutcome(ctx, nil, fmt.Errorf("decode device receipt: %w", err))
	}
	if !domain.ReceiptMatches(receipt, command, s.bootID) {
		return s.unknownDeviceOutcome(ctx, nil, errors.New("device receipt identity mismatch"))
	}
	return s.completeCommandOutcome(ctx, command, receipt, claim, commandIdentity)
}

func (s *Session) completeCommandOutcome(ctx context.Context, command domain.Command, receipt domain.Receipt, claim deviceauthority.TargetClaim, commandIdentity string) (*Exchange, bool, error) {
	partial := &Exchange{Receipt: receipt}
	if err := s.authority.AssertClaim(ctx, claim); err != nil {
		return s.unknownDeviceOutcome(ctx, partial, fmt.Errorf("authority lost during device exchange: %w", err))
	}
	result, err := s.receiveDeviceResult(ctx, command, receipt)
	if err != nil {
		return s.unknownDeviceOutcome(ctx, partial, err)
	}
	return s.cacheCommandOutcome(ctx, partial, result, claim, commandIdentity, command.IdempotencyKey)
}

func (s *Session) receiveDeviceResult(ctx context.Context, command domain.Command, receipt domain.Receipt) (domain.Result, error) {
	resultFrame, err := s.transport.Receive(ctx)
	if err != nil {
		return domain.Result{}, fmt.Errorf("receive device result: %w", err)
	}
	result, err := wire.DecodeResult(resultFrame)
	if err != nil {
		s.telemetry.ObserveDeviceFrameError()
		return domain.Result{}, fmt.Errorf("decode device result: %w", err)
	}
	if !domain.ResultMatches(result, command, s.bootID, receipt) {
		return domain.Result{}, errors.New("device result identity or status mismatch")
	}
	return result, nil
}

func (s *Session) cacheCommandOutcome(ctx context.Context, partial *Exchange, result domain.Result, claim deviceauthority.TargetClaim, commandIdentity, idempotencyKey string) (*Exchange, bool, error) {
	partial.Result = result
	if err := s.authority.AssertClaim(ctx, claim); err != nil {
		return s.unknownDeviceOutcome(ctx, partial, fmt.Errorf("authority lost during device result: %w", err))
	}
	s.receipts[idempotencyKey] = cachedExchange{commandIdentity: commandIdentity, exchange: partial.clone()}
	return partial, true, nil
}

func (s *Session) unknownDeviceOutcome(ctx context.Context, partial *Exchange, err error) (*Exchange, bool, error) {
	barrierErr := s.requireReconciliation(ctx, "device exchange was not trustworthy")
	// A malformed, incomplete, or mismatched pair leaves the next frame's
	// meaning unknowable. Do not let a caller reuse a potentially desynchronized
	// transport; a fresh handshake is required.
	s.invalidateTransportLocked()
	return partial, true, &deviceExchangeError{err: errors.Join(err, barrierErr)}
}
