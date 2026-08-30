package actions

import (
	"context"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// DeviceTransport is the typed gateway link used by the serial effector. The
// gateway owns raw serial framing, reconnect, and device identity; this
// interface carries only one validated device record at a time.
type DeviceTransport interface {
	// Send and Receive are one request/receipt pair. Callers must serialize the
	// full pair and must not overlap it with QueryState; DeviceSession provides
	// that serialization. QueryState serializes its own control write and read.
	Send(context.Context, []byte) error
	Receive(context.Context) ([]byte, error)
	// QueryState asks the gateway to obtain a fresh device.state record. The
	// gateway owns the request framing; callers must not satisfy this by
	// returning an unsolicited or cached frame.
	QueryState(context.Context) ([]byte, error)
	Close() error
}

// DeviceSessionConfig configures the state handshake and capability allowlist.
type DeviceSessionConfig struct {
	Transport                DeviceTransport
	Catalog                  *CapabilityCatalog
	AllowedCapabilityDigests []string
	AllowedFirmwareDigests   []string
	AuthorityEpoch           string
	OwnerInstance            string
	Authority                *storage.TargetAuthority
	Reconciliation           *storage.ReconciliationStore
	Telemetry                *telemetry.Runtime
}

// DeviceSession is the only action-plane object allowed to use a
// DeviceTransport. It serializes request/receipt exchanges and remembers
// accepted idempotency keys for the current device boot.
type DeviceSession struct {
	mu                     sync.Mutex
	transport              DeviceTransport
	catalog                *CapabilityCatalog
	authorityEpoch         string
	ownerInstance          string
	authority              *storage.TargetAuthority
	reconciliation         *storage.ReconciliationStore
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
	safeStopRequested      bool
	opened                 bool
	closed                 bool
}

// OpenDeviceSession performs the mandatory device.state handshake. No command
// can be exchanged until protocol, firmware, capability, and authority
// configuration are accepted.
func OpenDeviceSession(ctx context.Context, config DeviceSessionConfig) (*DeviceSession, error) {
	if err := validateSessionConfig(config); err != nil {
		return nil, err
	}
	config.OwnerInstance = resolveOwnerInstance(config)
	if err := validateOwnerInstance(config); err != nil {
		return nil, err
	}
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

	session := newDeviceSession(config)
	opened := false
	defer func() {
		if !opened {
			_ = config.Transport.Close()
		}
	}()
	state, err := session.receiveHandshake(ctx, catalogDigest)
	if err != nil {
		return nil, err
	}
	if err := session.bindHandshakeState(ctx, state); err != nil {
		return nil, err
	}
	session.opened = true
	opened = true
	return session, nil
}

func validateSessionConfig(config DeviceSessionConfig) error {
	if config.Transport == nil || config.Catalog == nil {
		return fmt.Errorf("device transport and capability catalog are required")
	}
	if config.AuthorityEpoch == "" {
		return fmt.Errorf("device authority epoch is required")
	}
	if config.Authority == nil || config.Reconciliation == nil {
		return fmt.Errorf("device target authority and reconciliation store are required")
	}
	if config.Authority.Owner == nil || config.Authority.EpochControl == nil {
		return fmt.Errorf("device target authority requires runtime owner and epoch control")
	}
	if len(config.AllowedFirmwareDigests) == 0 {
		return fmt.Errorf("device firmware allow-list is required")
	}
	if config.Authority.DB != config.Reconciliation.DB || config.Authority.DB != config.Authority.Owner.DB || config.Authority.DB != config.Authority.EpochControl.DB {
		return fmt.Errorf("device authority components must share one database")
	}
	return nil
}

func resolveOwnerInstance(config DeviceSessionConfig) string {
	if config.OwnerInstance != "" {
		return config.OwnerInstance
	}
	if config.Authority.InstanceID != "" {
		return config.Authority.InstanceID
	}
	return config.Authority.Owner.InstanceID
}

func validateOwnerInstance(config DeviceSessionConfig) error {
	if config.OwnerInstance == "" || config.Authority.Owner.InstanceID != config.OwnerInstance {
		return fmt.Errorf("device owner instance must match runtime owner")
	}
	if config.Authority.InstanceID != "" && config.Authority.InstanceID != config.OwnerInstance {
		return fmt.Errorf("device owner instance must match target authority")
	}
	return nil
}

func newDeviceSession(config DeviceSessionConfig) *DeviceSession {
	return &DeviceSession{
		transport: config.Transport, catalog: config.Catalog,
		authorityEpoch: config.AuthorityEpoch, ownerInstance: config.OwnerInstance,
		authority: config.Authority, reconciliation: config.Reconciliation,
		receipts: make(map[string]cachedReceipt), claimedTargets: make(map[string]struct{}),
		telemetry:              config.Telemetry,
		allowedFirmwareDigests: append([]string(nil), config.AllowedFirmwareDigests...),
	}
}

func (s *DeviceSession) receiveHandshake(ctx context.Context, catalogDigest string) (map[string]any, error) {
	frame, err := s.transport.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("receive device state handshake: %w", err)
	}
	state, err := validateDeviceState(frame, s.catalog, catalogDigest, []string{catalogDigest}, s.allowedFirmwareDigests)
	if err != nil {
		if s.telemetry != nil {
			s.telemetry.ObserveDeviceFrameError()
		}
		return nil, fmt.Errorf("validate device state handshake: %w", err)
	}
	return state, nil
}

func (s *DeviceSession) bindHandshakeState(ctx context.Context, state map[string]any) error {
	s.deviceID, s.bootID = stateString(state, "device_id"), stateString(state, "boot_id")
	s.firmwareDigest, s.capabilityDigest = stateString(state, "firmware_digest"), stateString(state, "capability_digest")
	s.safeState, _ = state["safe_state"].(bool)
	var err error
	s.stateDigest, err = stateDigest(state)
	if err != nil {
		return fmt.Errorf("digest device state handshake: %w", err)
	}
	if err := s.authority.AssertRuntime(ctx, s.authorityEpoch); err != nil {
		return fmt.Errorf("assert authority before binding device state: %w", err)
	}
	priorBarrier, err := s.reconciliation.Required(ctx, s.deviceID)
	if err != nil {
		return fmt.Errorf("read device reconciliation barrier: %w", err)
	}
	wasRequired := s.reconciliationRequired
	required, err := s.reconciliation.BindState(ctx, state, s.authorityEpoch, s.ownerInstance)
	if err != nil {
		return fmt.Errorf("bind device reconciliation state: %w", err)
	}
	s.reconciliationRequired = required
	s.stateQueryRequired = priorBarrier || required
	if !wasRequired && s.reconciliationRequired && s.telemetry != nil {
		s.telemetry.ObserveReconciliationBarrier()
	}
	s.safeStopRequested, err = s.authority.SafeStopRequested(ctx, s.deviceID, s.bootID)
	if err != nil {
		return fmt.Errorf("read durable safe-stop state: %w", err)
	}
	if s.safeState && s.telemetry != nil {
		s.telemetry.ObserveSafeStateEntry()
	}
	return nil
}
