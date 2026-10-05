package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

// SafeStop sends the catalog-owned safe-state command through the
// priority lane and returns its ordered receipt/result pair. It does not
// consult ordinary authority or the reconciliation barrier, and there is
// intentionally no method that clears a physical e-stop. The result is
// device-reported terminal status; query-state verification remains separate.
func (s *Session) SafeStop(ctx context.Context, target string) (Exchange, bool, error) {
	if s == nil {
		return Exchange{}, false, fmt.Errorf("device session is not open")
	}
	if _, ok := s.catalog.SafeStops[target]; !ok {
		return Exchange{}, false, fmt.Errorf("safe stop target %q is not cataloged", target)
	}
	s.latchSafeStop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return Exchange{}, false, err
	}
	return s.deliverSafeStop(ctx, target)
}

func (s *Session) latchSafeStop() {
	s.stopMu.Lock()
	s.safeStopLatched = true
	s.stopMu.Unlock()
}

// safeStopAttempt binds a priority command to its durable request stage.
type safeStopAttempt struct {
	command      domain.Command
	claim        deviceauthority.TargetClaim
	requestedErr error
}

// safeStopFailure carries the transport failure without losing send certainty.
type safeStopFailure struct {
	cause  error
	prefix string
	sent   bool
}

func (s *Session) deliverSafeStop(ctx context.Context, target string) (Exchange, bool, error) {
	command, err := s.catalog.MaterializeSafeStop(target, s.bootID)
	if err != nil {
		return Exchange{}, false, err
	}
	claim := s.targetClaim(target)
	requestedErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopRequested, map[string]any{
		"command_id": command.CommandID, "state_digest": s.stateDigest,
	})
	s.telemetry.ObserveSafeStopRequested()
	return s.sendSafeStop(ctx, safeStopAttempt{command: command, claim: claim, requestedErr: requestedErr})
}

func (s *Session) sendSafeStop(ctx context.Context, attempt safeStopAttempt) (Exchange, bool, error) {
	frame, err := wire.Encode(attempt.command.Document())
	if err != nil {
		return s.failedSafeStop(ctx, attempt, safeStopFailure{cause: fmt.Errorf("encode safe stop: %w", err), prefix: "safe-stop preparation failed"})
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		return s.failedSafeStop(ctx, attempt, safeStopFailure{cause: fmt.Errorf("send safe stop: %w", err), prefix: "safe-stop send failed", sent: transport.MayHaveSent(err)})
	}
	return s.completeSafeStopExchange(ctx, attempt)
}

func (s *Session) failedSafeStop(ctx context.Context, attempt safeStopAttempt, failure safeStopFailure) (Exchange, bool, error) {
	var barrierErr error
	if failure.sent {
		// A partial write may have crossed the gateway. Close the link before
		// doing durable bookkeeping so a later priority safe-stop cannot consume
		// an old response from this exchange.
		s.invalidateTransportLocked()
		barrierErr = s.requireReconciliation(ctx, "safe-stop outcome was not trustworthy")
	}
	return s.recordFailedSafeStop(ctx, attempt, failure, barrierErr)
}

func (s *Session) recordFailedSafeStop(ctx context.Context, attempt safeStopAttempt, failure safeStopFailure, barrierErr error) (Exchange, bool, error) {
	details := map[string]any{"error": failure.cause.Error()}
	if failure.sent {
		details["sent"] = true
	}
	recordErr := s.recordSafeStop(ctx, attempt.claim, deviceauthority.SafeStopFailed, details)
	s.telemetry.ObserveSafeStopFailure()
	if failure.sent {
		return Exchange{}, true, &deviceExchangeError{err: errors.Join(failure.cause, attempt.requestedErr, recordErr, barrierErr)}
	}
	return Exchange{}, false, fmt.Errorf("%s: %w", failure.prefix, errors.Join(failure.cause, attempt.requestedErr, recordErr))
}

func (s *Session) completeSafeStopExchange(ctx context.Context, attempt safeStopAttempt) (Exchange, bool, error) {
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		return s.failedSafeStopReceive(ctx, attempt, err)
	}
	receipt, err := wire.DecodeReceipt(reply)
	if err != nil || !domain.ReceiptMatches(receipt, attempt.command, s.bootID) {
		return s.failedSafeStopReceipt(ctx, attempt, nil, err)
	}
	result, resultErr := s.receiveDeviceResult(ctx, attempt.command, receipt)
	if resultErr != nil {
		return s.failedSafeStopReceipt(ctx, attempt, &Exchange{Receipt: receipt}, resultErr)
	}
	return s.concludeSafeStopResponse(ctx, attempt, Exchange{Receipt: receipt, Result: result})
}

func (s *Session) failedSafeStopReceive(ctx context.Context, attempt safeStopAttempt, err error) (Exchange, bool, error) {
	s.invalidateTransportLocked()
	barrierErr := s.requireReconciliation(ctx, "safe-stop receipt was not received")
	recordErr := s.recordSafeStop(ctx, attempt.claim, deviceauthority.SafeStopFailed, map[string]any{"error": err.Error(), "sent": true})
	s.telemetry.ObserveSafeStopFailure()
	return Exchange{}, true, &deviceExchangeError{err: errors.Join(err, attempt.requestedErr, recordErr, barrierErr)}
}

func (s *Session) failedSafeStopReceipt(ctx context.Context, attempt safeStopAttempt, partial *Exchange, decodeErr error) (Exchange, bool, error) {
	s.invalidateTransportLocked()
	barrierErr := s.requireReconciliation(ctx, "safe-stop exchange was not trustworthy")
	details := map[string]any{"sent": true}
	if decodeErr != nil {
		details["error"] = decodeErr.Error()
	} else {
		details["error"] = "safe-stop receipt identity mismatch"
	}
	recordErr := s.recordSafeStop(ctx, attempt.claim, deviceauthority.SafeStopFailed, details)
	s.telemetry.ObserveSafeStopFailure()
	return failedSafeStopResponse(partial, decodeErr, errors.Join(attempt.requestedErr, recordErr, barrierErr))
}

func failedSafeStopResponse(partial *Exchange, decodeErr, lifecycleErr error) (Exchange, bool, error) {
	if decodeErr == nil {
		decodeErr = errors.New("safe-stop receipt identity mismatch")
	}
	if partial == nil {
		partial = &Exchange{}
	}
	return *partial, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("decode safe-stop response: %w", decodeErr), lifecycleErr)}
}

func (s *Session) concludeSafeStopResponse(ctx context.Context, attempt safeStopAttempt, exchange Exchange) (Exchange, bool, error) {
	if !exchange.Receipt.Accepted {
		return s.rejectedSafeStop(ctx, attempt, exchange)
	}
	return s.recordCompletedSafeStop(ctx, attempt, exchange)
}

func (s *Session) rejectedSafeStop(ctx context.Context, attempt safeStopAttempt, exchange Exchange) (Exchange, bool, error) {
	recordErr := s.recordSafeStop(ctx, attempt.claim, deviceauthority.SafeStopFailed, map[string]any{
		"command_id": attempt.command.CommandID, "accepted": false,
		"result_status": exchange.Result.Status, "error_code": exchange.Result.Document["error_code"],
	})
	s.telemetry.ObserveSafeStopFailure()
	return s.classifySafeStopRejection(ctx, exchange, attempt.requestedErr, recordErr)
}

func (s *Session) classifySafeStopRejection(ctx context.Context, exchange Exchange, requestedErr, recordErr error) (Exchange, bool, error) {
	lifecycleErr := errors.Join(requestedErr, recordErr)
	if lifecycleErr != nil {
		s.invalidateTransportLocked()
		barrierErr := s.requireReconciliation(ctx, "safe-stop rejection evidence was not durable")
		return exchange, true, &actionport.UnknownOutcomeError{Err: errors.Join(errors.New("device rejected safe stop but lifecycle evidence was not durable"), lifecycleErr, barrierErr)}
	}
	return exchange, true, &deviceExchangeError{err: errors.Join(errors.New("device rejected safe stop"), requestedErr, recordErr)}
}

func (s *Session) recordCompletedSafeStop(ctx context.Context, attempt safeStopAttempt, exchange Exchange) (Exchange, bool, error) {
	completedErr := s.recordSafeStop(ctx, attempt.claim, deviceauthority.SafeStopCompleted, map[string]any{
		"command_id": attempt.command.CommandID, "accepted": exchange.Receipt.Accepted, "result_status": exchange.Result.Status,
	})
	if lifecycleErr := errors.Join(attempt.requestedErr, completedErr); lifecycleErr != nil {
		barrierErr := s.requireReconciliation(ctx, "safe-stop lifecycle evidence was not durable")
		s.telemetry.ObserveSafeStopFailure()
		return exchange, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("safe-stop accepted but lifecycle evidence was not durable: %w", lifecycleErr), barrierErr)}
	}
	s.telemetry.ObserveSafeStopCompleted()
	return exchange, true, nil
}

func (s *Session) recordSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, stage deviceauthority.SafeStopStage, details map[string]any) error {
	if err := s.authority.RecordSafeStop(ctx, claim, stage, details); err != nil {
		return fmt.Errorf("record safe-stop event: %w", err)
	}
	return nil
}

func (s *Session) isSafeStopLatched() bool {
	s.stopMu.RLock()
	defer s.stopMu.RUnlock()
	return s.safeStopLatched
}
