package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

const reconciliationPersistTimeout = 5 * time.Second

// BootID returns the current handshake-bound boot identity.
func (s *Session) BootID() string {
	return s.readString(func(s *Session) string { return s.bootID })
}

// CapabilityDigest returns the capability catalog digest accepted during the
// device handshake.
func (s *Session) CapabilityDigest() string {
	return s.readString(func(s *Session) string { return s.capabilityDigest })
}

func (s *Session) readString(read func(*Session) string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return read(s)
}

// WithTelemetry connects session protocol observations to runtime telemetry.
func (s *Session) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *Session {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.telemetry = runtimeTelemetry
	return s
}

// QueryState receives and validates one typed device.state record. A new boot
// opens the reconciliation barrier before the state is exposed for commands.
func (s *Session) QueryState(ctx context.Context) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return nil, fmt.Errorf("device session is not open")
	}
	return s.queryCurrentState(ctx)
}

func (s *Session) queryCurrentState(ctx context.Context) (map[string]any, error) {
	catalogDigest, err := s.catalog.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest device capability catalog: %w", err)
	}
	frame, err := s.transport.QueryState(ctx)
	if err != nil {
		return s.failStateRefresh(fmt.Errorf("receive device state refresh: %w", err))
	}
	state, err := validateDeviceState(frame, s.catalog, catalogDigest, []string{catalogDigest}, s.allowedFirmwareDigests)
	if err != nil {
		return s.failStateRefresh(fmt.Errorf("validate device state refresh: %w", err))
	}
	return s.acceptCurrentState(ctx, state)
}

func (s *Session) acceptCurrentState(ctx context.Context, state map[string]any) (map[string]any, error) {
	if stateString(state, "device_id") != s.deviceID {
		s.opened = false
		return nil, fmt.Errorf("device identity changed from %q to %q", s.deviceID, stateString(state, "device_id"))
	}
	if err := s.applyRefreshedState(ctx, state); err != nil {
		return nil, err
	}
	return cloneDocument(state), nil
}

func (s *Session) applyRefreshedState(ctx context.Context, state map[string]any) error {
	previousSafeState := s.safeState
	if bootID := stateString(state, "boot_id"); bootID != s.bootID {
		s.bootID = bootID
		s.receipts = make(map[string]cachedReceipt)
		s.reconciliationRequired = true
	}
	s.firmwareDigest = stateString(state, "firmware_digest")
	s.capabilityDigest = stateString(state, "capability_digest")
	s.safeState, _ = state["safe_state"].(bool)
	var err error
	s.stateDigest, err = domain.StateDigest(state)
	if err != nil {
		return fmt.Errorf("digest device state refresh: %w", err)
	}
	return s.completeStateRefresh(ctx, state, previousSafeState)
}

func (s *Session) completeStateRefresh(ctx context.Context, state map[string]any, previousSafeState bool) error {
	if err := s.bindRefreshedState(ctx, state); err != nil {
		return err
	}
	if !previousSafeState && s.safeState {
		s.telemetry.ObserveSafeStateEntry()
	}
	return nil
}

func (s *Session) bindRefreshedState(ctx context.Context, state map[string]any) error {
	if err := s.authority.AssertRuntime(ctx, s.ownerEpoch); err != nil {
		s.reconciliationRequired = true
		s.opened = false
		return fmt.Errorf("assert authority before binding refreshed state: %w", err)
	}
	return s.persistRefreshedState(ctx, state)
}

func (s *Session) persistRefreshedState(ctx context.Context, state map[string]any) error {
	wasRequired := s.reconciliationRequired
	required, err := s.authority.RecordDeviceState(ctx, s.owner(), state)
	if err != nil {
		// A refresh is a durable safety transition. If its barrier write cannot
		// be proven, this session is no longer safe to use.
		s.reconciliationRequired = true
		s.opened = false
		return fmt.Errorf("bind refreshed device state: %w", err)
	}
	s.reconciliationRequired = required
	s.stateQueryRequired = false
	if !wasRequired && s.reconciliationRequired {
		s.telemetry.ObserveReconciliationBarrier()
	}
	return nil
}

func (s *Session) failStateRefresh(err error) (map[string]any, error) {
	s.telemetry.ObserveDeviceFrameError()
	s.opened = false
	return nil, err
}

// invalidateTransportLocked makes the current session unusable after a
// partial or invalid wire exchange. The caller holds s.mu; Close can still be
// called later to release claims and perform its normal cleanup.
func (s *Session) invalidateTransportLocked() {
	s.opened = false
	if s.transport != nil {
		_ = s.transport.Close()
	}
}
