package actions

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// DeviceTransport is the typed gateway link used by the serial effector. The
// gateway owns raw serial framing, reconnect, and device identity; this
// interface carries only one validated device record at a time.
type DeviceTransport interface {
	Send(context.Context, []byte) error
	Receive(context.Context) ([]byte, error)
	Close() error
}

// DeviceSessionConfig configures the state handshake and capability allowlist.
type DeviceSessionConfig struct {
	Transport                DeviceTransport
	Catalog                  *CapabilityCatalog
	AllowedCapabilityDigests []string
	AllowedFirmwareDigests   []string
	AuthorityEpoch           string
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
	deviceID               string
	bootID                 string
	firmwareDigest         string
	capabilityDigest       string
	safeState              bool
	telemetry              *telemetry.Runtime
	allowedFirmwareDigests []string
	receipts               map[string]cachedReceipt
	opened                 bool
	closed                 bool
}

// OpenDeviceSession performs the mandatory device.state handshake. No command
// can be exchanged until protocol, firmware, capability, and authority
// configuration are accepted.
func OpenDeviceSession(ctx context.Context, config DeviceSessionConfig) (*DeviceSession, error) {
	if config.Transport == nil || config.Catalog == nil {
		return nil, fmt.Errorf("device transport and capability catalog are required")
	}
	if config.AuthorityEpoch == "" {
		return nil, fmt.Errorf("device authority epoch is required")
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
	session := &DeviceSession{
		transport: config.Transport, catalog: config.Catalog,
		authorityEpoch: config.AuthorityEpoch, receipts: make(map[string]cachedReceipt),
		telemetry:              config.Telemetry,
		allowedFirmwareDigests: append([]string(nil), config.AllowedFirmwareDigests...),
	}
	frame, err := config.Transport.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("receive device state handshake: %w", err)
	}
	state, err := validateDeviceState(frame, config.Catalog, catalogDigest, config.AllowedCapabilityDigests, config.AllowedFirmwareDigests)
	if err != nil {
		if config.Telemetry != nil {
			config.Telemetry.ObserveDeviceFrameError()
		}
		return nil, fmt.Errorf("validate device state handshake: %w", err)
	}
	deviceID := stateString(state, "device_id")
	bootID := stateString(state, "boot_id")
	session.deviceID, session.bootID = deviceID, bootID
	session.firmwareDigest, session.capabilityDigest = stateString(state, "firmware_digest"), stateString(state, "capability_digest")
	session.safeState, _ = state["safe_state"].(bool)
	if session.safeState && session.telemetry != nil {
		session.telemetry.ObserveSafeStateEntry()
	}
	session.opened = true
	return session, nil
}

// DeviceID returns the handshake-bound device identity.
func (s *DeviceSession) DeviceID() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deviceID
}

// BootID returns the current handshake-bound boot identity.
func (s *DeviceSession) BootID() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bootID
}

// AuthorityEpoch returns the runtime authority epoch bound to the session.
func (s *DeviceSession) AuthorityEpoch() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authorityEpoch
}

// CapabilityDigest returns the capability catalog digest accepted during the
// device handshake.
func (s *DeviceSession) CapabilityDigest() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.capabilityDigest
}

// SafeState reports the last safe_state value received from the device. It is
// informational here; firmware and the gateway remain responsible for the
// device's real watchdog and safe-state behavior.
func (s *DeviceSession) SafeState() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.safeState
}

// WithTelemetry connects session protocol observations to runtime telemetry.
func (s *DeviceSession) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *DeviceSession {
	if s != nil {
		s.telemetry = runtimeTelemetry
	}
	return s
}

// RefreshState reads another device.state record. A changed boot invalidates
// cached receipts before the new boot is accepted, so a prior semantic effect
// cannot be mistaken for a receipt from the new device lifetime.
func (s *DeviceSession) RefreshState(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return fmt.Errorf("device session is not open")
	}
	catalogDigest, err := s.catalog.Digest()
	if err != nil {
		return fmt.Errorf("digest device capability catalog: %w", err)
	}
	frame, err := s.transport.Receive(ctx)
	if err != nil {
		s.telemetry.ObserveDeviceFrameError()
		s.opened = false
		return fmt.Errorf("receive device state refresh: %w", err)
	}
	state, err := validateDeviceState(frame, s.catalog, catalogDigest, []string{catalogDigest}, s.allowedFirmwareDigests)
	if err != nil {
		s.telemetry.ObserveDeviceFrameError()
		s.opened = false
		return fmt.Errorf("validate device state refresh: %w", err)
	}
	if stateString(state, "device_id") != s.deviceID {
		s.opened = false
		return fmt.Errorf("device identity changed from %q to %q", s.deviceID, stateString(state, "device_id"))
	}
	previousSafeState := s.safeState
	if bootID := stateString(state, "boot_id"); bootID != s.bootID {
		s.bootID = bootID
		s.receipts = make(map[string]cachedReceipt)
	}
	s.firmwareDigest = stateString(state, "firmware_digest")
	s.capabilityDigest = stateString(state, "capability_digest")
	s.safeState, _ = state["safe_state"].(bool)
	if !previousSafeState && s.safeState {
		s.telemetry.ObserveSafeStateEntry()
	}
	return nil
}

// Exchange sends one already-materialized command and waits for its receipt.
// The bool reports whether bytes were handed to the transport; callers must
// treat a post-send receive error as an unknown outcome.
func (s *DeviceSession) Exchange(ctx context.Context, command map[string]any) (map[string]any, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return nil, false, fmt.Errorf("device session is not open")
	}
	frame, err := EncodeDeviceRecord(command)
	if err != nil {
		return nil, false, err
	}
	semanticDigest, err := semanticCommandDigest(command)
	if err != nil {
		return nil, false, fmt.Errorf("digest device command identity: %w", err)
	}
	idempotencyKey, _ := command["idempotency_key"].(string)
	if cached, ok := s.receipts[idempotencyKey]; ok {
		if cached.commandDigest != semanticDigest {
			return nil, false, fmt.Errorf("idempotency key %q conflicts with the prior device command", idempotencyKey)
		}
		return cloneDocument(cached.receipt), true, nil
	}
	expectedBoot, _ := command["expected_boot_id"].(string)
	if expectedBoot != s.bootID {
		return nil, false, fmt.Errorf("device boot changed: command expects %q, session is %q", expectedBoot, s.bootID)
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		return nil, false, fmt.Errorf("send device command: %w", err)
	}
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		return nil, true, &deviceExchangeError{err: err}
	}
	receipt, err := DecodeDeviceRecord(reply)
	if err != nil {
		if s.telemetry != nil {
			s.telemetry.ObserveDeviceFrameError()
		}
		return nil, true, &deviceExchangeError{err: fmt.Errorf("decode device receipt: %w", err)}
	}
	if receipt["message_type"] != "receipt" || receipt["command_id"] != command["command_id"] || receipt["boot_id"] != s.bootID {
		return nil, true, &deviceExchangeError{err: errors.New("device receipt identity mismatch")}
	}
	s.receipts[idempotencyKey] = cachedReceipt{commandDigest: semanticDigest, receipt: cloneDocument(receipt)}
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
	if s.transport == nil {
		return nil
	}
	return s.transport.Close()
}

func validateDeviceState(frame []byte, catalog *CapabilityCatalog, catalogDigest string, allowedCapabilityDigests, allowedFirmwareDigests []string) (map[string]any, error) {
	state, err := DecodeDeviceRecord(frame)
	if err != nil {
		return nil, err
	}
	if state["message_type"] != "state" {
		return nil, fmt.Errorf("device handshake returned %v, want state", state["message_type"])
	}
	if protocol := documentInt64(state, "protocol_version"); protocol != int64(catalog.ProtocolVersion) {
		return nil, fmt.Errorf("device protocol version %d is unsupported", protocol)
	}
	capabilityDigest := stateString(state, "capability_digest")
	if capabilityDigest != catalogDigest || !contains(allowedCapabilityDigests, capabilityDigest) {
		return nil, fmt.Errorf("device capability digest %q does not match the allow-listed catalog", capabilityDigest)
	}
	firmwareDigest := stateString(state, "firmware_digest")
	if len(allowedFirmwareDigests) > 0 && !contains(allowedFirmwareDigests, firmwareDigest) {
		return nil, fmt.Errorf("device firmware digest %q is not allow-listed", firmwareDigest)
	}
	if stateString(state, "device_id") == "" || stateString(state, "boot_id") == "" {
		return nil, fmt.Errorf("device handshake identity is incomplete")
	}
	return state, nil
}

func stateString(state map[string]any, key string) string {
	value, _ := state[key].(string)
	return value
}

type cachedReceipt struct {
	commandDigest string
	receipt       map[string]any
}

type deviceExchangeError struct{ err error }

func (e *deviceExchangeError) Error() string { return e.err.Error() }
func (e *deviceExchangeError) Unwrap() error { return e.err }

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func cloneDocument(document map[string]any) map[string]any {
	clone := make(map[string]any, len(document))
	for key, value := range document {
		clone[key] = value
	}
	return clone
}

func semanticCommandDigest(command map[string]any) (string, error) {
	identity := cloneDocument(command)
	delete(identity, "command_id")
	return canonicaljson.Digest(canonicaljson.DomainCommand, identity)
}
