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

func (s *Session) deliverSafeStop(ctx context.Context, target string) (Exchange, bool, error) {
	command, err := s.catalog.MaterializeSafeStop(target, s.bootID)
	if err != nil {
		return Exchange{}, false, err
	}
	claim := s.targetClaim(target)
	requestedErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopRequested, map[string]any{
		"command_id": command["command_id"], "state_digest": s.stateDigest,
	})
	s.telemetry.ObserveSafeStopRequested()
	return s.sendSafeStop(ctx, command, claim, requestedErr)
}

func (s *Session) sendSafeStop(ctx context.Context, command map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (Exchange, bool, error) {
	frame, err := wire.Encode(command)
	if err != nil {
		return s.failedSafeStop(ctx, claim, requestedErr, fmt.Errorf("encode safe stop: %w", err), "safe-stop preparation failed", false)
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		return s.failedSafeStop(ctx, claim, requestedErr, fmt.Errorf("send safe stop: %w", err), "safe-stop send failed", transport.MayHaveSent(err))
	}
	return s.completeSafeStopExchange(ctx, command, claim, requestedErr)
}

func (s *Session) failedSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr, cause error, prefix string, sent bool) (Exchange, bool, error) {
	var barrierErr error
	if sent {
		// A partial write may have crossed the gateway. Close the link before
		// doing durable bookkeeping so a later priority safe-stop cannot consume
		// an old response from this exchange.
		s.invalidateTransportLocked()
		barrierErr = s.requireReconciliation(ctx, "safe-stop outcome was not trustworthy")
	}
	return s.recordFailedSafeStop(ctx, claim, requestedErr, cause, barrierErr, prefix, sent)
}

func (s *Session) recordFailedSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr, cause, barrierErr error, prefix string, sent bool) (Exchange, bool, error) {
	details := map[string]any{"error": cause.Error()}
	if sent {
		details["sent"] = true
	}
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, details)
	s.telemetry.ObserveSafeStopFailure()
	if sent {
		return Exchange{}, true, &deviceExchangeError{err: errors.Join(cause, requestedErr, recordErr, barrierErr)}
	}
	return Exchange{}, false, fmt.Errorf("%s: %w", prefix, errors.Join(cause, requestedErr, recordErr))
}

func (s *Session) completeSafeStopExchange(ctx context.Context, command map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (Exchange, bool, error) {
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		return s.failedSafeStopReceive(ctx, claim, requestedErr, err)
	}
	receipt, err := wire.Decode(reply)
	if err != nil || !domain.ReceiptMatches(receipt, command, s.bootID) {
		return s.failedSafeStopReceipt(ctx, claim, requestedErr, nil, err)
	}
	result, resultErr := s.receiveDeviceResult(ctx, command, receipt)
	if resultErr != nil {
		return s.failedSafeStopReceipt(ctx, claim, requestedErr, &Exchange{Receipt: receipt}, resultErr)
	}
	return s.concludeSafeStopResponse(ctx, command, receipt, result, claim, requestedErr)
}

func (s *Session) failedSafeStopReceive(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr, err error) (Exchange, bool, error) {
	s.invalidateTransportLocked()
	barrierErr := s.requireReconciliation(ctx, "safe-stop receipt was not received")
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, map[string]any{"error": err.Error(), "sent": true})
	s.telemetry.ObserveSafeStopFailure()
	return Exchange{}, true, &deviceExchangeError{err: errors.Join(err, requestedErr, recordErr, barrierErr)}
}

func (s *Session) failedSafeStopReceipt(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr error, partial *Exchange, decodeErr error) (Exchange, bool, error) {
	s.invalidateTransportLocked()
	barrierErr := s.requireReconciliation(ctx, "safe-stop exchange was not trustworthy")
	details := map[string]any{"sent": true}
	if decodeErr != nil {
		details["error"] = decodeErr.Error()
	} else {
		details["error"] = "safe-stop receipt identity mismatch"
	}
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, details)
	s.telemetry.ObserveSafeStopFailure()
	return failedSafeStopResponse(partial, decodeErr, requestedErr, recordErr, barrierErr)
}

func failedSafeStopResponse(partial *Exchange, decodeErr, requestedErr, recordErr, barrierErr error) (Exchange, bool, error) {
	if decodeErr == nil {
		decodeErr = errors.New("safe-stop receipt identity mismatch")
	}
	if partial == nil {
		partial = &Exchange{}
	}
	return *partial, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("decode safe-stop response: %w", decodeErr), requestedErr, recordErr, barrierErr)}
}

func (s *Session) concludeSafeStopResponse(ctx context.Context, command, receipt, result map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (Exchange, bool, error) {
	accepted, _ := receipt["accepted"].(bool)
	if !accepted {
		return s.rejectedSafeStop(ctx, command, receipt, result, claim, requestedErr)
	}
	return s.recordCompletedSafeStop(ctx, claim, command, receipt, result, requestedErr)
}

func (s *Session) rejectedSafeStop(ctx context.Context, command, receipt, result map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (Exchange, bool, error) {
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, map[string]any{
		"command_id": command["command_id"], "accepted": false,
		"result_status": result["status"], "error_code": result["error_code"],
	})
	s.telemetry.ObserveSafeStopFailure()
	return s.classifySafeStopRejection(ctx, receipt, result, requestedErr, recordErr)
}

func (s *Session) classifySafeStopRejection(ctx context.Context, receipt, result map[string]any, requestedErr, recordErr error) (Exchange, bool, error) {
	lifecycleErr := errors.Join(requestedErr, recordErr)
	if lifecycleErr != nil {
		s.invalidateTransportLocked()
		barrierErr := s.requireReconciliation(ctx, "safe-stop rejection evidence was not durable")
		return Exchange{Receipt: receipt, Result: result}, true, &actionport.UnknownOutcomeError{Err: errors.Join(errors.New("device rejected safe stop but lifecycle evidence was not durable"), lifecycleErr, barrierErr)}
	}
	return Exchange{Receipt: receipt, Result: result}, true, &deviceExchangeError{err: errors.Join(errors.New("device rejected safe stop"), requestedErr, recordErr)}
}

func (s *Session) recordCompletedSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, command, receipt, result map[string]any, requestedErr error) (Exchange, bool, error) {
	completedErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopCompleted, map[string]any{
		"command_id": command["command_id"], "accepted": receipt["accepted"], "result_status": result["status"],
	})
	if lifecycleErr := errors.Join(requestedErr, completedErr); lifecycleErr != nil {
		barrierErr := s.requireReconciliation(ctx, "safe-stop lifecycle evidence was not durable")
		s.telemetry.ObserveSafeStopFailure()
		return Exchange{Receipt: receipt, Result: result}, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("safe-stop accepted but lifecycle evidence was not durable: %w", lifecycleErr), barrierErr)}
	}
	s.telemetry.ObserveSafeStopCompleted()
	return Exchange{Receipt: receipt, Result: result}, true, nil
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
