package device

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

// SafeStopWithResult sends the catalog-owned safe-state command through the
// priority lane and returns its ordered receipt/result pair. It does not
// consult ordinary authority or the reconciliation barrier, and there is
// intentionally no method that clears a physical e-stop. The result is
// device-reported terminal status; query-state verification remains separate.
func (s *DeviceSession) SafeStopWithResult(ctx context.Context, target string) (DeviceExchange, bool, error) {
	if s == nil {
		return DeviceExchange{}, false, fmt.Errorf("device session is not open")
	}
	if _, ok := s.catalog.SafeStops[target]; !ok {
		return DeviceExchange{}, false, fmt.Errorf("safe stop target %q is not cataloged", target)
	}
	s.requestSafeStop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return DeviceExchange{}, false, err
	}
	return s.deliverSafeStop(ctx, target)
}

func (s *DeviceSession) requestSafeStop() {
	s.stopMu.Lock()
	s.safeStopRequested = true
	s.stopMu.Unlock()
}

func (s *DeviceSession) deliverSafeStop(ctx context.Context, target string) (DeviceExchange, bool, error) {
	command, err := s.catalog.MaterializeSafeStop(target, s.bootID)
	if err != nil {
		return DeviceExchange{}, false, err
	}
	claim := s.targetClaim(target)
	requestedErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopRequested, map[string]any{
		"command_id": command["command_id"], "state_digest": s.stateDigest,
	})
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopRequested()
	}
	return s.sendSafeStop(ctx, command, claim, requestedErr)
}

func (s *DeviceSession) sendSafeStop(ctx context.Context, command map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (DeviceExchange, bool, error) {
	frame, err := EncodeDeviceRecord(command)
	if err != nil {
		return s.failedSafeStop(ctx, claim, requestedErr, fmt.Errorf("encode safe stop: %w", err), "safe-stop preparation failed", false)
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		return s.failedSafeStop(ctx, claim, requestedErr, fmt.Errorf("send safe stop: %w", err), "safe-stop send failed", transportMayHaveSent(err))
	}
	return s.completeSafeStopExchange(ctx, command, claim, requestedErr)
}

func (s *DeviceSession) failedSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr, cause error, prefix string, sent bool) (DeviceExchange, bool, error) {
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

func (s *DeviceSession) recordFailedSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr, cause, barrierErr error, prefix string, sent bool) (DeviceExchange, bool, error) {
	details := map[string]any{"error": cause.Error()}
	if sent {
		details["sent"] = true
	}
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, details)
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopFailure()
	}
	if sent {
		return DeviceExchange{}, true, &deviceExchangeError{err: errors.Join(cause, requestedErr, recordErr, barrierErr)}
	}
	return DeviceExchange{}, false, fmt.Errorf("%s: %w", prefix, errors.Join(cause, requestedErr, recordErr))
}

func (s *DeviceSession) completeSafeStopExchange(ctx context.Context, command map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (DeviceExchange, bool, error) {
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		return s.failedSafeStopReceive(ctx, claim, requestedErr, err)
	}
	receipt, err := DecodeDeviceRecord(reply)
	if err != nil || !domain.ReceiptMatches(receipt, command, s.bootID) {
		return s.failedSafeStopReceipt(ctx, claim, requestedErr, nil, err)
	}
	result, resultErr := s.receiveDeviceResult(ctx, command, receipt)
	if resultErr != nil {
		return s.failedSafeStopReceipt(ctx, claim, requestedErr, &DeviceExchange{Receipt: receipt}, resultErr)
	}
	return s.concludeSafeStopResponse(ctx, command, receipt, result, claim, requestedErr)
}

func (s *DeviceSession) failedSafeStopReceive(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr, err error) (DeviceExchange, bool, error) {
	s.invalidateTransportLocked()
	barrierErr := s.requireReconciliation(ctx, "safe-stop receipt was not received")
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, map[string]any{"error": err.Error(), "sent": true})
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopFailure()
	}
	return DeviceExchange{}, true, &deviceExchangeError{err: errors.Join(err, requestedErr, recordErr, barrierErr)}
}

func (s *DeviceSession) failedSafeStopReceipt(ctx context.Context, claim deviceauthority.TargetClaim, requestedErr error, partial *DeviceExchange, decodeErr error) (DeviceExchange, bool, error) {
	s.invalidateTransportLocked()
	barrierErr := s.requireReconciliation(ctx, "safe-stop exchange was not trustworthy")
	details := map[string]any{"sent": true}
	if decodeErr != nil {
		details["error"] = decodeErr.Error()
	} else {
		details["error"] = "safe-stop receipt identity mismatch"
	}
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, details)
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopFailure()
	}
	return failedSafeStopResponse(partial, decodeErr, requestedErr, recordErr, barrierErr)
}

func failedSafeStopResponse(partial *DeviceExchange, decodeErr, requestedErr, recordErr, barrierErr error) (DeviceExchange, bool, error) {
	if decodeErr == nil {
		decodeErr = errors.New("safe-stop receipt identity mismatch")
	}
	if partial == nil {
		partial = &DeviceExchange{}
	}
	return *partial, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("decode safe-stop response: %w", decodeErr), requestedErr, recordErr, barrierErr)}
}

func (s *DeviceSession) concludeSafeStopResponse(ctx context.Context, command, receipt, result map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (DeviceExchange, bool, error) {
	accepted, _ := receipt["accepted"].(bool)
	if !accepted {
		return s.rejectedSafeStop(ctx, command, receipt, result, claim, requestedErr)
	}
	return s.recordCompletedSafeStop(ctx, claim, command, receipt, result, requestedErr)
}

func (s *DeviceSession) rejectedSafeStop(ctx context.Context, command, receipt, result map[string]any, claim deviceauthority.TargetClaim, requestedErr error) (DeviceExchange, bool, error) {
	recordErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopFailed, map[string]any{
		"command_id": command["command_id"], "accepted": false,
		"result_status": result["status"], "error_code": result["error_code"],
	})
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopFailure()
	}
	return s.classifySafeStopRejection(ctx, receipt, result, requestedErr, recordErr)
}

func (s *DeviceSession) classifySafeStopRejection(ctx context.Context, receipt, result map[string]any, requestedErr, recordErr error) (DeviceExchange, bool, error) {
	lifecycleErr := errors.Join(requestedErr, recordErr)
	if lifecycleErr != nil {
		s.invalidateTransportLocked()
		barrierErr := s.requireReconciliation(ctx, "safe-stop rejection evidence was not durable")
		return DeviceExchange{Receipt: receipt, Result: result}, true, &actionport.UnknownOutcomeError{Err: errors.Join(errors.New("device rejected safe stop but lifecycle evidence was not durable"), lifecycleErr, barrierErr)}
	}
	return DeviceExchange{Receipt: receipt, Result: result}, true, &deviceExchangeError{err: errors.Join(errors.New("device rejected safe stop"), requestedErr, recordErr)}
}

func (s *DeviceSession) recordCompletedSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, command, receipt, result map[string]any, requestedErr error) (DeviceExchange, bool, error) {
	completedErr := s.recordSafeStop(ctx, claim, deviceauthority.SafeStopCompleted, map[string]any{
		"command_id": command["command_id"], "accepted": receipt["accepted"], "result_status": result["status"],
	})
	if lifecycleErr := errors.Join(requestedErr, completedErr); lifecycleErr != nil {
		barrierErr := s.requireReconciliation(ctx, "safe-stop lifecycle evidence was not durable")
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return DeviceExchange{Receipt: receipt, Result: result}, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("safe-stop accepted but lifecycle evidence was not durable: %w", lifecycleErr), barrierErr)}
	}
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopCompleted()
	}
	return DeviceExchange{Receipt: receipt, Result: result}, true, nil
}

func (s *DeviceSession) recordSafeStop(ctx context.Context, claim deviceauthority.TargetClaim, stage deviceauthority.SafeStopStage, details map[string]any) error {
	if s.authority == nil {
		return fmt.Errorf("safe-stop lifecycle store is not configured")
	}
	if err := s.authority.RecordSafeStop(ctx, claim, stage, details); err != nil {
		return fmt.Errorf("record safe-stop event: %w", err)
	}
	return nil
}

func (s *DeviceSession) stopRequested() bool {
	s.stopMu.RLock()
	defer s.stopMu.RUnlock()
	return s.safeStopRequested
}
