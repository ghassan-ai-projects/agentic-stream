package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/domain"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Transport is the typed gateway link used by the serial effector. The
// gateway owns raw serial framing, reconnect, and device identity; this
// interface carries one validated device record at a time and preserves the
// receipt/result ordering required by Session.
type Transport interface {
	// Send and Receive are one request/receipt/result exchange. Callers must
	// serialize the full exchange and must not overlap it with QueryState;
	// Session provides that serialization. QueryState serializes its own
	// control write and read.
	Send(context.Context, []byte) error
	Receive(context.Context) ([]byte, error)
	// QueryState asks the gateway to obtain a fresh device.state record. The
	// gateway owns the request framing; callers must not satisfy this by
	// returning an unsolicited or cached frame.
	QueryState(context.Context) ([]byte, error)
	Close() error
}

// SessionConfig configures the state handshake and capability allowlist.
type SessionConfig struct {
	Transport                Transport
	Catalog                  *domain.CapabilityCatalog
	AllowedCapabilityDigests []string
	AllowedFirmwareDigests   []string
	OwnerEpoch               string
	OwnerInstance            string
	Authority                *deviceauthority.Service
	Telemetry                *telemetry.Runtime
}

// Session is the only action-plane object allowed to use a
// Transport. It serializes request/receipt exchanges and remembers
// accepted idempotency keys for the current device boot.
type Session struct {
	mu                     sync.Mutex
	transport              Transport
	catalog                *domain.CapabilityCatalog
	ownerEpoch             string
	ownerInstance          string
	authority              *deviceauthority.Service
	deviceID               string
	bootID                 string
	firmwareDigest         string
	capabilityDigest       string
	safeState              bool
	telemetry              *telemetry.Runtime
	allowedFirmwareDigests []string
	receipts               map[string]cachedReceipt
	claimedTargets         map[string]struct{}
	stateDigest            string
	reconciliationRequired bool
	stateQueryRequired     bool
	stopMu                 sync.RWMutex
	safeStopLatched        bool
	opened                 bool
	closed                 bool
}

// OpenSession performs the mandatory device.state handshake. No command
// can be exchanged until protocol, firmware, capability, and authority
// configuration are accepted.
func OpenSession(ctx context.Context, config SessionConfig) (*Session, error) {
	if err := validateSessionConfig(config); err != nil {
		return nil, err
	}
	config.OwnerInstance = resolveOwnerInstance(config)
	if err := validateOwnerInstance(config); err != nil {
		return nil, err
	}
	return openCatalogSession(ctx, config)
}

func validateSessionConfig(config SessionConfig) error {
	if config.Transport == nil || config.Catalog == nil {
		return fmt.Errorf("device transport and capability catalog are required")
	}
	if config.OwnerEpoch == "" {
		return fmt.Errorf("device owner epoch is required")
	}
	if config.Authority == nil {
		return fmt.Errorf("device authority is required")
	}
	if len(config.AllowedFirmwareDigests) == 0 {
		return fmt.Errorf("device firmware allow-list is required")
	}
	return nil
}

func resolveOwnerInstance(config SessionConfig) string {
	if config.OwnerInstance != "" {
		return config.OwnerInstance
	}
	return config.Authority.OwnerInstance()
}

func validateOwnerInstance(config SessionConfig) error {
	if config.OwnerInstance == "" || config.Authority.OwnerInstance() != config.OwnerInstance {
		return fmt.Errorf("device owner instance must match runtime owner")
	}
	return nil
}

func openCatalogSession(ctx context.Context, config SessionConfig) (*Session, error) {
	if len(config.AllowedCapabilityDigests) == 0 {
		return nil, fmt.Errorf("device capability allow-list is required")
	}
	catalogDigest, err := config.Catalog.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest device capability catalog: %w", err)
	}
	if !contains(config.AllowedCapabilityDigests, catalogDigest) {
		return nil, fmt.Errorf("capability catalog digest is not allow-listed")
	}

	return handshakeDeviceSession(ctx, config, catalogDigest)
}

func handshakeDeviceSession(ctx context.Context, config SessionConfig, catalogDigest string) (*Session, error) {
	session := newDeviceSession(config)
	defer func() {
		if !session.opened {
			_ = config.Transport.Close()
		}
	}()
	return session.acceptHandshake(ctx, catalogDigest)
}

func newDeviceSession(config SessionConfig) *Session {
	return &Session{
		transport: config.Transport, catalog: config.Catalog,
		ownerEpoch: config.OwnerEpoch, ownerInstance: config.OwnerInstance,
		authority: config.Authority,
		receipts:  make(map[string]cachedReceipt), claimedTargets: make(map[string]struct{}),
		telemetry:              config.Telemetry,
		allowedFirmwareDigests: append([]string(nil), config.AllowedFirmwareDigests...),
	}
}

func (s *Session) acceptHandshake(ctx context.Context, catalogDigest string) (*Session, error) {
	state, err := s.receiveHandshake(ctx, catalogDigest)
	if err != nil {
		return nil, err
	}
	if err := s.bindHandshakeState(ctx, state); err != nil {
		return nil, err
	}
	s.opened = true
	return s, nil
}

func (s *Session) receiveHandshake(ctx context.Context, catalogDigest string) (map[string]any, error) {
	frame, err := s.transport.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("receive device state handshake: %w", err)
	}
	state, err := validateDeviceState(frame, s.catalog, catalogDigest, []string{catalogDigest}, s.allowedFirmwareDigests)
	if err != nil {
		s.telemetry.ObserveDeviceFrameError()
		return nil, fmt.Errorf("validate device state handshake: %w", err)
	}
	return state, nil
}

func (s *Session) bindHandshakeState(ctx context.Context, state map[string]any) error {
	s.deviceID, s.bootID = stateString(state, "device_id"), stateString(state, "boot_id")
	s.firmwareDigest, s.capabilityDigest = stateString(state, "firmware_digest"), stateString(state, "capability_digest")
	s.safeState, _ = state["safe_state"].(bool)
	var err error
	s.stateDigest, err = domain.StateDigest(state)
	if err != nil {
		return fmt.Errorf("digest device state handshake: %w", err)
	}
	return s.bindHandshakeBarrier(ctx, state)
}

func (s *Session) bindHandshakeBarrier(ctx context.Context, state map[string]any) error {
	if err := s.authority.AssertRuntime(ctx, s.ownerEpoch); err != nil {
		return fmt.Errorf("assert authority before binding device state: %w", err)
	}
	priorBarrier, err := s.authority.ReconciliationRequired(ctx, s.deviceID)
	if err != nil {
		return fmt.Errorf("read device reconciliation barrier: %w", err)
	}
	return s.persistHandshakeBarrier(ctx, state, priorBarrier)
}

func (s *Session) persistHandshakeBarrier(ctx context.Context, state map[string]any, priorBarrier bool) error {
	wasRequired := s.reconciliationRequired
	required, err := s.authority.RecordDeviceState(ctx, s.owner(), state)
	if err != nil {
		return fmt.Errorf("bind device reconciliation state: %w", err)
	}
	s.reconciliationRequired = required
	s.stateQueryRequired = priorBarrier || required
	if !wasRequired && s.reconciliationRequired {
		s.telemetry.ObserveReconciliationBarrier()
	}
	return s.restoreSafeStopState(ctx)
}

func (s *Session) restoreSafeStopState(ctx context.Context) error {
	var err error
	s.safeStopLatched, err = s.authority.SafeStopLatched(ctx, s.deviceBoot())
	if err != nil {
		return fmt.Errorf("read durable safe-stop state: %w", err)
	}
	if s.safeState {
		s.telemetry.ObserveSafeStateEntry()
	}
	return nil
}
