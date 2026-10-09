package app

import (
	"context"
	"fmt"
	"maps"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

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
func (s *Session) QueryState(ctx context.Context) (domain.State, error) {
	if s == nil {
		return domain.State{}, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return domain.State{}, fmt.Errorf("device session is not open")
	}
	return s.queryCurrentState(ctx)
}

func (s *Session) queryCurrentState(ctx context.Context) (domain.State, error) {
	catalogDigest, err := s.catalog.Digest()
	if err != nil {
		return domain.State{}, fmt.Errorf("digest device capability catalog: %w", err)
	}
	frame, err := s.transport.QueryState(ctx)
	if err != nil {
		return s.failStateRefresh(fmt.Errorf("receive device state refresh: %w", err))
	}
	state, err := decodeState(frame, s.catalog, catalogDigest, s.allowedFirmwareDigests)
	if err != nil {
		return s.failStateRefresh(fmt.Errorf("validate device state refresh: %w", err))
	}
	return s.acceptCurrentState(ctx, state)
}

func (s *Session) acceptCurrentState(ctx context.Context, state domain.State) (domain.State, error) {
	if state.DeviceID != s.deviceID {
		s.opened = false
		return domain.State{}, fmt.Errorf("device identity changed from %q to %q", s.deviceID, state.DeviceID)
	}
	if err := s.applyRefreshedState(ctx, state); err != nil {
		return domain.State{}, err
	}
	state.Document = maps.Clone(state.Document)
	return state, nil
}

// applyRefreshedState adopts a newer state. A new boot discards cached
// receipts and requires reconciliation before the state is used.
func (s *Session) applyRefreshedState(ctx context.Context, state domain.State) error {
	previousSafeState := s.safeState
	if state.BootID != s.bootID {
		s.bootID = state.BootID
		s.receipts = make(map[string]cachedExchange)
		s.reconciliationRequired = true
	}
	s.firmwareDigest, s.capabilityDigest = state.FirmwareDigest, state.CapabilityDigest
	s.safeState = state.SafeState
	var err error
	s.stateDigest, err = state.Digest()
	if err != nil {
		return fmt.Errorf("digest device state refresh: %w", err)
	}
	return s.completeStateRefresh(ctx, state, previousSafeState)
}

func (s *Session) completeStateRefresh(ctx context.Context, state domain.State, previousSafeState bool) error {
	if err := s.persistRefreshedState(ctx, state); err != nil {
		return err
	}
	if !previousSafeState && s.safeState {
		s.telemetry.ObserveSafeStateEntry()
	}
	return nil
}

func (s *Session) persistRefreshedState(ctx context.Context, state domain.State) error {
	wasRequired := s.reconciliationRequired
	required, err := s.authority.RecordDeviceState(ctx, s.owner(), state.Document)
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

func (s *Session) failStateRefresh(err error) (domain.State, error) {
	s.telemetry.ObserveDeviceFrameError()
	s.opened = false
	return domain.State{}, err
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
