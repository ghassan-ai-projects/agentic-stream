package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// DeviceTransport is the typed gateway link used by the serial effector. The
// gateway owns raw serial framing, reconnect, and device identity; this
// interface carries only one validated device record at a time.
type DeviceTransport interface {
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
	if config.Transport == nil || config.Catalog == nil {
		return nil, fmt.Errorf("device transport and capability catalog are required")
	}
	if config.AuthorityEpoch == "" {
		return nil, fmt.Errorf("device authority epoch is required")
	}
	if config.Authority == nil || config.Reconciliation == nil {
		return nil, fmt.Errorf("device target authority and reconciliation store are required")
	}
	if config.Authority.Owner == nil || config.Authority.EpochControl == nil {
		return nil, fmt.Errorf("device target authority requires runtime owner and epoch control")
	}
	if config.Authority.DB != config.Reconciliation.DB || config.Authority.DB != config.Authority.Owner.DB || config.Authority.DB != config.Authority.EpochControl.DB {
		return nil, fmt.Errorf("device authority components must share one database")
	}
	if config.OwnerInstance == "" && config.Authority != nil {
		config.OwnerInstance = config.Authority.Owner.InstanceID
		if config.Authority.InstanceID != "" {
			config.OwnerInstance = config.Authority.InstanceID
		}
	}
	if config.OwnerInstance == "" || config.Authority.Owner.InstanceID != config.OwnerInstance {
		return nil, fmt.Errorf("device owner instance must match runtime owner")
	}
	if config.Authority.InstanceID != "" && config.Authority.InstanceID != config.OwnerInstance {
		return nil, fmt.Errorf("device owner instance must match target authority")
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
		authorityEpoch: config.AuthorityEpoch, ownerInstance: config.OwnerInstance,
		authority: config.Authority, reconciliation: config.Reconciliation,
		receipts: make(map[string]cachedReceipt), claimedTargets: make(map[string]struct{}),
		telemetry:              config.Telemetry,
		allowedFirmwareDigests: append([]string(nil), config.AllowedFirmwareDigests...),
	}
	opened := false
	defer func() {
		if !opened {
			_ = config.Transport.Close()
		}
	}()
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
	session.stateDigest, err = stateDigest(state)
	if err != nil {
		return nil, fmt.Errorf("digest device state handshake: %w", err)
	}
	if config.Reconciliation != nil {
		if err := config.Authority.AssertRuntime(ctx, config.AuthorityEpoch); err != nil {
			return nil, fmt.Errorf("assert authority before binding device state: %w", err)
		}
		priorBarrier, err := config.Reconciliation.Required(ctx, deviceID)
		if err != nil {
			return nil, fmt.Errorf("read device reconciliation barrier: %w", err)
		}
		wasRequired := session.reconciliationRequired
		session.reconciliationRequired, err = config.Reconciliation.BindState(ctx, state, config.AuthorityEpoch, config.OwnerInstance)
		if err != nil {
			return nil, fmt.Errorf("bind device reconciliation state: %w", err)
		}
		if !wasRequired && session.reconciliationRequired && session.telemetry != nil {
			session.telemetry.ObserveReconciliationBarrier()
		}
		session.stateQueryRequired = priorBarrier || session.reconciliationRequired
	}
	session.safeStopRequested, err = config.Authority.SafeStopRequested(ctx, deviceID, bootID)
	if err != nil {
		return nil, fmt.Errorf("read durable safe-stop state: %w", err)
	}
	if session.safeState && session.telemetry != nil {
		session.telemetry.ObserveSafeStateEntry()
	}
	session.opened = true
	opened = true
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

// ReconciliationRequired reports whether the device boot barrier blocks
// ordinary energizing commands.
func (s *DeviceSession) ReconciliationRequired() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconciliationRequired
}

// WithTelemetry connects session protocol observations to runtime telemetry.
func (s *DeviceSession) WithTelemetry(runtimeTelemetry *telemetry.Runtime) *DeviceSession {
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.telemetry = runtimeTelemetry
	}
	return s
}

// RefreshState reads another device.state record. A changed boot invalidates
// cached receipts before the new boot is accepted, so a prior semantic effect
// cannot be mistaken for a receipt from the new device lifetime.
func (s *DeviceSession) RefreshState(ctx context.Context) error {
	_, err := s.QueryState(ctx)
	return err
}

// QueryState receives and validates one typed device.state record. A new boot
// opens the reconciliation barrier before the state is exposed for commands.
func (s *DeviceSession) QueryState(ctx context.Context) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return nil, fmt.Errorf("device session is not open")
	}
	catalogDigest, err := s.catalog.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest device capability catalog: %w", err)
	}
	frame, err := s.transport.QueryState(ctx)
	if err != nil {
		if s.telemetry != nil {
			s.telemetry.ObserveDeviceFrameError()
		}
		s.opened = false
		return nil, fmt.Errorf("receive device state refresh: %w", err)
	}
	state, err := validateDeviceState(frame, s.catalog, catalogDigest, []string{catalogDigest}, s.allowedFirmwareDigests)
	if err != nil {
		if s.telemetry != nil {
			s.telemetry.ObserveDeviceFrameError()
		}
		s.opened = false
		return nil, fmt.Errorf("validate device state refresh: %w", err)
	}
	if stateString(state, "device_id") != s.deviceID {
		s.opened = false
		return nil, fmt.Errorf("device identity changed from %q to %q", s.deviceID, stateString(state, "device_id"))
	}
	previousSafeState := s.safeState
	if bootID := stateString(state, "boot_id"); bootID != s.bootID {
		s.bootID = bootID
		s.receipts = make(map[string]cachedReceipt)
		s.reconciliationRequired = true
	}
	s.firmwareDigest = stateString(state, "firmware_digest")
	s.capabilityDigest = stateString(state, "capability_digest")
	s.safeState, _ = state["safe_state"].(bool)
	s.stateDigest, err = stateDigest(state)
	if err != nil {
		return nil, fmt.Errorf("digest device state refresh: %w", err)
	}
	if s.reconciliation != nil {
		if err := s.authority.AssertRuntime(ctx, s.authorityEpoch); err != nil {
			s.reconciliationRequired = true
			s.opened = false
			return nil, fmt.Errorf("assert authority before binding refreshed state: %w", err)
		}
		wasRequired := s.reconciliationRequired
		required, bindErr := s.reconciliation.BindState(ctx, state, s.authorityEpoch, s.ownerInstance)
		if bindErr != nil {
			// A state refresh is a durable safety transition. If its barrier
			// write cannot be proven, this session is no longer safe to use.
			s.reconciliationRequired = true
			s.opened = false
			return nil, fmt.Errorf("bind refreshed device state: %w", bindErr)
		}
		s.reconciliationRequired = required
		s.stateQueryRequired = false
		if !wasRequired && s.reconciliationRequired && s.telemetry != nil {
			s.telemetry.ObserveReconciliationBarrier()
		}
	}
	if !previousSafeState && s.safeState {
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStateEntry()
		}
	}
	return cloneDocument(state), nil
}

// ResolveReconciliation records typed state/feedback evidence for the device
// barrier. Unknown command outcomes must already have gone through the
// dispatcher reconciliation path; the durable store refuses to clear while
// command ledgers remain unresolved. Manual review leaves the barrier closed.
func (s *DeviceSession) ResolveReconciliation(ctx context.Context, finalStatus string, evidence map[string]any) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("device session is not open")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return false, fmt.Errorf("device session is not open")
	}
	if !s.reconciliationRequired {
		return false, fmt.Errorf("device reconciliation barrier is not open")
	}
	if s.stateQueryRequired {
		return false, fmt.Errorf("device state query is required before reconciliation can be resolved")
	}
	if evidence == nil || evidence["state_digest"] != s.stateDigest {
		return false, fmt.Errorf("reconciliation evidence must bind the latest device state digest")
	}
	if err := s.authority.AssertRuntime(ctx, s.authorityEpoch); err != nil {
		return false, fmt.Errorf("assert reconciliation authority: %w", err)
	}
	cleared, err := s.reconciliation.Resolve(ctx, s.deviceID, s.bootID, finalStatus, evidence, s.authorityEpoch, s.ownerInstance)
	if err != nil {
		return false, fmt.Errorf("resolve device reconciliation: %w", err)
	}
	if cleared {
		s.reconciliationRequired = false
	}
	return cleared, nil
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
	if s.stopRequested() {
		return nil, false, fmt.Errorf("safe stop has priority over ordinary device commands")
	}
	frame, err := EncodeDeviceRecord(command)
	if err != nil {
		return nil, false, fmt.Errorf("encode device command: %w", err)
	}
	semanticDigest, err := semanticCommandDigest(command)
	if err != nil {
		return nil, false, fmt.Errorf("digest device command identity: %w", err)
	}
	idempotencyKey, _ := command["idempotency_key"].(string)
	expectedBoot, _ := command["expected_boot_id"].(string)
	if expectedBoot != s.bootID {
		return nil, false, fmt.Errorf("device boot changed: command expects %q, session is %q", expectedBoot, s.bootID)
	}
	if s.reconciliationRequired {
		return nil, false, storage.ErrReconciliationRequired
	}
	target, _ := command["target"].(string)
	claim := storage.TargetClaim{Target: target, DeviceID: s.deviceID, BootID: s.bootID, AuthorityEpoch: s.authorityEpoch, OwnerInstance: s.ownerInstance}
	if s.authority != nil {
		if err := s.authority.Claim(ctx, claim); err != nil {
			return nil, false, fmt.Errorf("claim device target: %w", err)
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
			return nil, false, fmt.Errorf("bind device command: %w", errors.Join(err, releaseErr))
		}
		s.claimedTargets[target] = struct{}{}
		// Claim performs the durable admission check. Repeat it directly before
		// transport delivery to minimize the revoke-to-send race; a post-send
		// failure remains an unknown outcome because bytes cannot be retracted.
		if err := s.authority.Assert(ctx, claim); err != nil {
			return nil, false, fmt.Errorf("assert device target authority: %w", err)
		}
	}
	if cached, ok := s.receipts[idempotencyKey]; ok {
		if cached.commandDigest != semanticDigest {
			return nil, false, fmt.Errorf("idempotency key %q conflicts with the prior device command", idempotencyKey)
		}
		return cloneDocument(cached.receipt), true, nil
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
	if s.authority != nil {
		if err := s.authority.Assert(ctx, claim); err != nil {
			return nil, true, &deviceExchangeError{err: fmt.Errorf("authority lost during device exchange: %w", err)}
		}
	}
	s.receipts[idempotencyKey] = cachedReceipt{commandDigest: semanticDigest, receipt: cloneDocument(receipt)}
	return receipt, true, nil
}

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
	s.stopMu.Lock()
	s.safeStopRequested = true
	s.stopMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened || s.closed {
		return nil, false, fmt.Errorf("device session is not open")
	}
	command, err := s.catalog.MaterializeSafeStop(target, s.bootID)
	if err != nil {
		return nil, false, err
	}
	claim := storage.TargetClaim{
		Target: target, DeviceID: s.deviceID, BootID: s.bootID,
		AuthorityEpoch: s.authorityEpoch, OwnerInstance: s.ownerInstance,
	}
	requestedErr := s.recordSafeStop(ctx, claim, "safe_stop_requested", map[string]any{
		"command_id": command["command_id"], "state_digest": s.stateDigest,
	})
	if s.telemetry != nil {
		s.telemetry.ObserveSafeStopRequested()
	}
	frame, err := EncodeDeviceRecord(command)
	if err != nil {
		recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", map[string]any{"error": err.Error()})
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return nil, false, fmt.Errorf("safe-stop preparation failed: %w", errors.Join(fmt.Errorf("encode safe stop: %w", err), requestedErr, recordErr))
	}
	if err := s.transport.Send(ctx, frame); err != nil {
		recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", map[string]any{"error": err.Error()})
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return nil, false, fmt.Errorf("safe-stop send failed: %w", errors.Join(fmt.Errorf("send safe stop: %w", err), requestedErr, recordErr))
	}
	reply, err := s.transport.Receive(ctx)
	if err != nil {
		recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", map[string]any{"error": err.Error(), "sent": true})
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return nil, true, &deviceExchangeError{err: errors.Join(err, requestedErr, recordErr)}
	}
	receipt, err := DecodeDeviceRecord(reply)
	if err != nil || receipt["message_type"] != "receipt" || receipt["command_id"] != command["command_id"] || receipt["boot_id"] != s.bootID {
		details := map[string]any{"sent": true}
		if err != nil {
			details["error"] = err.Error()
		} else {
			details["error"] = "safe-stop receipt identity mismatch"
		}
		recordErr := s.recordSafeStop(ctx, claim, "safe_stop_failed", details)
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		if err == nil {
			err = errors.New("safe-stop receipt identity mismatch")
		}
		return nil, true, &deviceExchangeError{err: errors.Join(fmt.Errorf("decode safe-stop receipt: %w", err), requestedErr, recordErr)}
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
	completedErr := s.recordSafeStop(ctx, claim, "safe_stop_completed", map[string]any{
		"command_id": command["command_id"], "accepted": receipt["accepted"],
	})
	if lifecycleErr := errors.Join(requestedErr, completedErr); lifecycleErr != nil {
		if s.telemetry != nil {
			s.telemetry.ObserveSafeStopFailure()
		}
		return receipt, true, &deviceExchangeError{err: fmt.Errorf("safe-stop accepted but lifecycle evidence was not durable: %w", lifecycleErr)}
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
	var releaseErr error
	if s.authority != nil {
		for target := range s.claimedTargets {
			err := s.authority.Release(context.Background(), storage.TargetClaim{Target: target, DeviceID: s.deviceID, BootID: s.bootID, AuthorityEpoch: s.authorityEpoch, OwnerInstance: s.ownerInstance})
			if errors.Is(err, storage.ErrTargetClaimNotOwned) {
				continue
			}
			releaseErr = errors.Join(releaseErr, err)
		}
	}
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
	digest, err := canonicaljson.Digest(canonicaljson.DomainCommand, identity)
	if err != nil {
		return "", fmt.Errorf("digest semantic device command: %w", err)
	}
	return digest, nil
}

func stateDigest(state map[string]any) (string, error) {
	data, err := canonicaljson.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("canonicalize device state: %w", err)
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}
