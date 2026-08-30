package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SafeStop sends the catalog-owned safe-state command through a priority path.
// It does not consult the ordinary authority or reconciliation barrier, and
// there is intentionally no method that clears a physical e-stop.
func (s *DeviceSession) SafeStop(ctx context.Context, target string) (map[string]any, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("device session is not open")
	}
	if _, ok := s.catalog.SafeStops[target]; !ok {
		return nil, false, fmt.Errorf("safe stop target %q is not cataloged", target)
	}
	s.requestSafeStop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpen(); err != nil {
		return nil, false, err
	}
	command, err := s.catalog.MaterializeSafeStop(target, s.bootID)
	if err != nil {
		return nil, false, err
	}
	claim := storage.TargetClaim{Target: target, DeviceID: s.deviceID, BootID: s.bootID, AuthorityEpoch: s.authorityEpoch, OwnerInstance: s.ownerInstance}
	requestedErr := s.recordSafeStop(ctx, claim, "safe_stop_requested", map[string]any{
		"command_id": command["command_id"], "state_digest": s.stateDigest,
	})
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopRequested()
	}
	frame, err := EncodeDeviceRecord(command)
	if err != nil {
		return s.failedSafeStop(ctx, claim, requestedErr, fmt.Errorf("encode safe stop: %w", err), "safe-stop preparation failed", false)
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		return s.failedSafeStop(ctx, claim, requestedErr, fmt.Errorf("send safe stop: %w", err), "safe-stop send failed", transportMayHaveSent(err))
	}
	return s.completeSafeStopExchange(ctx, command, claim, requestedErr)
}

func (s *DeviceSession) requestSafeStop() {
	s.stopMu.Lock()
	s.safeStopRequested = true
	s.stopMu.Unlock()
}

func (s *DeviceSession) failedSafeStop(ctx context.Context, claim storage.TargetClaim, requestedErr, cause error, prefix string, sent bool) (map[string]any, bool, error) {
	var barrierErr error
	if sent {
		barrierErr = s.requireReconciliation(ctx, "safe-stop outcome was not trustworthy")
	}
	details := map[string]any{"error": cause.Error()}
	if sent {
		details["sent"] = true
	}
	recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", details)
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopFailure()
	}
	if sent {
		return nil, true, &deviceExchangeError{err: errors.Join(cause, requestedErr, recordErr, barrierErr)}
	}
	return nil, false, fmt.Errorf("%s: %w", prefix, errors.Join(cause, requestedErr, recordErr))
}

func (s *DeviceSession) completeSafeStopExchange(ctx context.Context, command map[string]any, claim storage.TargetClaim, requestedErr error) (map[string]any, bool, error) {
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		barrierErr := s.requireReconciliation(ctx, "safe-stop receipt was not received")
		recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", map[string]any{"error": err.Error(), "sent": true})
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return nil, true, &deviceExchangeError{err: errors.Join(err, requestedErr, recordErr, barrierErr)}
	}
	receipt, err := DecodeDeviceRecord(reply)
	if err != nil || !receiptMatchesCommand(receipt, command, s.bootID) {
		return s.failedSafeStopReceipt(ctx, claim, requestedErr, err)
	}
	accepted, _ := receipt["accepted"].(bool)
	if !accepted {
		recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", map[string]any{
			"command_id": command["command_id"], "accepted": false,
		})
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return nil, true, &deviceExchangeError{err: errors.Join(errors.New("device rejected safe stop"), requestedErr, recordErr)}
	}
	return s.recordCompletedSafeStop(ctx, claim, command, receipt, requestedErr)
}

func (s *DeviceSession) failedSafeStopReceipt(ctx context.Context, claim storage.TargetClaim, requestedErr error, decodeErr error) (map[string]any, bool, error) {
	barrierErr := s.requireReconciliation(ctx, "safe-stop receipt was not trustworthy")
	details := map[string]any{"sent": true}
	if decodeErr != nil {
		details["error"] = decodeErr.Error()
	} else {
		details["error"] = "safe-stop receipt identity mismatch"
	}
	recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", details)
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopFailure()
	}
	if decodeErr == nil {
		decodeErr = errors.New("safe-stop receipt identity mismatch")
	}
	return nil, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("decode safe-stop receipt: %w", decodeErr), requestedErr, recordErr, barrierErr)}
}

func (s *DeviceSession) recordCompletedSafeStop(ctx context.Context, claim storage.TargetClaim, command, receipt map[string]any, requestedErr error) (map[string]any, bool, error) {
	completedErr := s.recordSafeStop(ctx, claim, "safe_stop_completed", map[string]any{
		"command_id": command["command_id"], "accepted": receipt["accepted"],
	})
	if lifecycleErr := errors.Join(requestedErr, completedErr); lifecycleErr != nil {
		barrierErr := s.requireReconciliation(ctx, "safe-stop lifecycle evidence was not durable")
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return receipt, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("safe-stop accepted but lifecycle evidence was not durable: %w", lifecycleErr), barrierErr)}
	}
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopCompleted()
	}
	return receipt, true, nil
}

// Close releases the gateway link. It is safe to call more than once.
func (s *DeviceSession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	releaseErr := s.releaseClaims()
	if s.transport == nil {
		if releaseErr != nil {
			return fmt.Errorf("release device target claims: %w", releaseErr)
		}
		return nil
	}
	if err := errors.Join(releaseErr, s.transport.Close()); err != nil {
		return fmt.Errorf("close device session: %w", err)
	}
	return nil
}

func (s *DeviceSession) releaseClaims() error {
	if s.authority == nil {
		return nil
	}
	var releaseErr error
	for target := range s.claimedTargets {
		err := s.authority.Release(context.Background(), storage.TargetClaim{Target: target, DeviceID: s.deviceID, BootID: s.bootID, AuthorityEpoch: s.authorityEpoch, OwnerInstance: s.ownerInstance})
		if errors.Is(err, storage.ErrTargetClaimNotOwned) {
			continue
		}
		releaseErr = errors.Join(releaseErr, err)
	}
	// Close adds the operation context at the public boundary.
	return releaseErr //nolint:wrapcheck // Close wraps the joined release errors.
}

func (s *DeviceSession) stopRequested() bool {
	s.stopMu.RLock()
	defer s.stopMu.RUnlock()
	return s.safeStopRequested
}

func (s *DeviceSession) recordSafeStop(ctx context.Context, claim storage.TargetClaim, eventType string, details map[string]any) error {
	if s.authority != nil {
		if err := s.authority.RecordSafeStop(ctx, claim, eventType, details); err != nil {
			return fmt.Errorf("record safe-stop event: %w", err)
		}
		return nil
	}
	if s.reconciliation != nil {
		if err := s.reconciliation.RecordSafeStop(ctx, claim.Target, claim.DeviceID, claim.BootID, claim.AuthorityEpoch, claim.OwnerInstance, eventType, details); err != nil {
			return fmt.Errorf("record safe-stop event: %w", err)
		}
		return nil
	}
	return fmt.Errorf("safe-stop lifecycle store is not configured")
}
