package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// DeviceExchange is the ordered terminal response to one device command.
// Receipt proves admission; Result is the device-reported terminal execution
// status. Neither is physical confirmation by itself.
type DeviceExchange struct {
	Receipt map[string]any
	Result  map[string]any
}

// Exchange sends one already-materialized command and consumes its receipt and
// terminal result. The bool reports whether bytes were handed to the transport;
// callers must treat a post-send receive error as an unknown outcome.
func (s *DeviceSession) Exchange(ctx context.Context, command map[string]any) (map[string]any, bool, error) {
	exchange, sent, err := s.exchange(ctx, command)
	if exchange == nil {
		return nil, sent, err
	}
	return exchange.Receipt, sent, err
}

// ExchangeWithResult is the result-bearing form of Exchange used by the
// action plane when it must persist both admission and terminal execution
// evidence.
func (s *DeviceSession) ExchangeWithResult(ctx context.Context, command map[string]any) (DeviceExchange, bool, error) {
	exchange, sent, err := s.exchange(ctx, command)
	if exchange == nil {
		return DeviceExchange{}, sent, err
	}
	return *exchange, sent, err
}

func (s *DeviceSession) exchange(ctx context.Context, command map[string]any) (*DeviceExchange, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nil, false, err
	}
	if s.stopRequested() {
		return nil, false, fmt.Errorf("safe stop has priority over ordinary device commands")
	}
	frame, semanticDigest, idempotencyKey, err := prepareCommand(command)
	if err != nil {
		return nil, false, err
	}
	if err := s.validateCommandLifetime(command); err != nil {
		return nil, false, err
	}
	if s.reconciliationRequired {
		return nil, false, storage.ErrReconciliationRequired
	}
	claim, err := s.claimCommand(ctx, command, semanticDigest)
	if err != nil {
		return nil, false, err
	}
	if receipt, ok, err := s.cachedReceipt(idempotencyKey, semanticDigest); ok || err != nil {
		if err != nil {
			return nil, ok, err
		}
		return &DeviceExchange{Receipt: receipt, Result: cloneDocument(s.receipts[idempotencyKey].result)}, ok, nil
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		sent := transportMayHaveSent(err)
		if sent {
			return s.unknownDeviceOutcome(ctx, nil, errors.Join(err, fmt.Errorf("command send may have crossed the gateway")))
		}
		return nil, sent, fmt.Errorf("send device command: %w", err)
	}
	return s.receiveCommandOutcome(ctx, command, claim, semanticDigest, idempotencyKey)
}

func (s *DeviceSession) ensureOpen() error {
	if !s.opened || s.closed {
		return fmt.Errorf("device session is not open")
	}
	return nil
}

func prepareCommand(command map[string]any) ([]byte, string, string, error) {
	frame, err := EncodeDeviceRecord(command)
	if err != nil {
		return nil, "", "", fmt.Errorf("encode device command: %w", err)
	}
	semanticDigest, err := semanticCommandDigest(command)
	if err != nil {
		return nil, "", "", fmt.Errorf("digest device command identity: %w", err)
	}
	idempotencyKey, _ := command["idempotency_key"].(string)
	return frame, semanticDigest, idempotencyKey, nil
}

func (s *DeviceSession) validateCommandLifetime(command map[string]any) error {
	expectedBoot, _ := command["expected_boot_id"].(string)
	if expectedBoot != s.bootID {
		return fmt.Errorf("device boot changed: command expects %q, session is %q", expectedBoot, s.bootID)
	}
	return nil
}

func (s *DeviceSession) claimCommand(ctx context.Context, command map[string]any, semanticDigest string) (storage.TargetClaim, error) {
	target, _ := command["target"].(string)
	claim := storage.TargetClaim{Target: target, DeviceID: s.deviceID, BootID: s.bootID, AuthorityEpoch: s.authorityEpoch, OwnerInstance: s.ownerInstance}
	if s.authority == nil {
		return claim, nil
	}
	if err := s.authority.Claim(ctx, claim); err != nil {
		return claim, fmt.Errorf("claim device target: %w", err)
	}
	if err := s.authority.BindCommand(ctx, storage.CommandBinding{
		CommandID: documentString(command, "command_id"), Target: target, DeviceID: s.deviceID,
		BootID: s.bootID, AuthorityEpoch: s.authorityEpoch, OwnerInstance: s.ownerInstance,
		CommandDigest: semanticDigest,
	}); err != nil {
		releaseErr := s.authority.Release(ctx, claim)
		if errors.Is(releaseErr, storage.ErrTargetClaimNotOwned) {
			releaseErr = nil
		}
		return claim, fmt.Errorf("bind device command: %w", errors.Join(err, releaseErr))
	}
	s.claimedTargets[target] = struct{}{}
	// Claim performs the durable admission check. Repeat it directly before
	// transport delivery to minimize the revoke-to-send race; a post-send
	// failure remains an unknown outcome because bytes cannot be retracted.
	if err := s.authority.Assert(ctx, claim); err != nil {
		return claim, fmt.Errorf("assert device target authority: %w", err)
	}
	return claim, nil
}

func (s *DeviceSession) cachedReceipt(idempotencyKey, semanticDigest string) (map[string]any, bool, error) {
	cached, ok := s.receipts[idempotencyKey]
	if !ok {
		return nil, false, nil
	}
	if cached.commandDigest != semanticDigest {
		return nil, false, fmt.Errorf("idempotency key %q conflicts with the prior device command", idempotencyKey)
	}
	return cloneDocument(cached.receipt), true, nil
}

func (s *DeviceSession) receiveCommandOutcome(ctx context.Context, command map[string]any, claim storage.TargetClaim, semanticDigest, idempotencyKey string) (*DeviceExchange, bool, error) {
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		return s.unknownDeviceOutcome(ctx, nil, err)
	}
	receipt, err := DecodeDeviceRecord(reply)
	if err != nil {
		if s.telemetry != nil {
			s.telemetry.ObserveDeviceFrameError()
		}
		return s.unknownDeviceOutcome(ctx, nil, fmt.Errorf("decode device receipt: %w", err))
	}
	if !receiptMatchesCommand(receipt, command, s.bootID) {
		return s.unknownDeviceOutcome(ctx, nil, errors.New("device receipt identity mismatch"))
	}
	partial := &DeviceExchange{Receipt: receipt}
	if s.authority != nil {
		if err := s.authority.Assert(ctx, claim); err != nil {
			return s.unknownDeviceOutcome(ctx, partial, fmt.Errorf("authority lost during device exchange: %w", err))
		}
	}
	result, err := s.receiveDeviceResult(ctx, command, receipt)
	if err != nil {
		return s.unknownDeviceOutcome(ctx, partial, err)
	}
	partial.Result = result
	if s.authority != nil {
		if err := s.authority.Assert(ctx, claim); err != nil {
			return s.unknownDeviceOutcome(ctx, partial, fmt.Errorf("authority lost during device result: %w", err))
		}
	}
	s.receipts[idempotencyKey] = cachedReceipt{commandDigest: semanticDigest, receipt: cloneDocument(receipt), result: cloneDocument(result)}
	return &DeviceExchange{Receipt: receipt, Result: result}, true, nil
}

func (s *DeviceSession) receiveDeviceResult(ctx context.Context, command, receipt map[string]any) (map[string]any, error) {
	resultFrame, err := s.transport.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("receive device result: %w", err)
	}
	result, err := DecodeDeviceRecord(resultFrame)
	if err != nil {
		if s.telemetry != nil {
			s.telemetry.ObserveDeviceFrameError()
		}
		return nil, fmt.Errorf("decode device result: %w", err)
	}
	if !resultMatchesCommand(result, command, s.bootID, receipt) {
		return nil, errors.New("device result identity or status mismatch")
	}
	return result, nil
}

func (s *DeviceSession) unknownDeviceOutcome(ctx context.Context, partial *DeviceExchange, err error) (*DeviceExchange, bool, error) {
	barrierErr := s.requireReconciliation(ctx, "device exchange was not trustworthy")
	// A malformed, incomplete, or mismatched pair leaves the next frame's
	// meaning unknowable. Do not let a caller reuse a potentially desynchronized
	// transport; a fresh handshake is required.
	s.invalidateTransportLocked()
	return partial, true, &deviceExchangeError{err: errors.Join(err, barrierErr)}
}

func receiptMatchesCommand(receipt, command map[string]any, bootID string) bool {
	return receipt["message_type"] == "receipt" &&
		receipt["command_id"] == command["command_id"] &&
		receipt["boot_id"] == bootID
}

func resultMatchesCommand(result, command map[string]any, bootID string, receipt map[string]any) bool {
	if result["message_type"] != "result" || result["command_id"] != command["command_id"] || result["boot_id"] != bootID {
		return false
	}
	status, _ := result["status"].(string)
	if status == "" {
		return false
	}
	accepted, _ := receipt["accepted"].(bool)
	if accepted {
		operation, _ := command["operation"].(string)
		if operation == "safe_stop" {
			return status == "safe_state" && result["error_code"] == nil
		}
		return status == "executed" && result["error_code"] == nil
	}
	return status == "rejected" && result["error_code"] == receipt["reject_code"]
}
