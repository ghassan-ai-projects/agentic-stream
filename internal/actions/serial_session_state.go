package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

const reconciliationPersistTimeout = 5 * time.Second

// DeviceID returns the handshake-bound device identity.
func (s *DeviceSession) DeviceID() string {
	return s.readString(func(s *DeviceSession) string { return s.deviceID })
}

// BootID returns the current handshake-bound boot identity.
func (s *DeviceSession) BootID() string {
	return s.readString(func(s *DeviceSession) string { return s.bootID })
}

// AuthorityEpoch returns the runtime authority epoch bound to the session.
func (s *DeviceSession) AuthorityEpoch() string {
	return s.readString(func(s *DeviceSession) string { return s.authorityEpoch })
}

// CapabilityDigest returns the capability catalog digest accepted during the
// device handshake.
func (s *DeviceSession) CapabilityDigest() string {
	return s.readString(func(s *DeviceSession) string { return s.capabilityDigest })
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
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.telemetry = runtimeTelemetry
	return s
}

func (s *DeviceSession) readString(read func(*DeviceSession) string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return read(s)
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
		return s.failStateRefresh(fmt.Errorf("receive device state refresh: %w", err))
	}
	state, err := validateDeviceState(frame, s.catalog, catalogDigest, []string{catalogDigest}, s.allowedFirmwareDigests)
	if err != nil {
		return s.failStateRefresh(fmt.Errorf("validate device state refresh: %w", err))
	}
	if stateString(state, "device_id") != s.deviceID {
		s.opened = false
		return nil, fmt.Errorf("device identity changed from %q to %q", s.deviceID, stateString(state, "device_id"))
	}
	if err := s.applyRefreshedState(ctx, state); err != nil {
		return nil, err
	}
	return cloneDocument(state), nil
}

// QueryStateEvidence performs one fresh device-state query and packages the
// state with the identity and digest fields required by durable reconciliation.
func (s *DeviceSession) QueryStateEvidence(ctx context.Context) (map[string]any, error) {
	state, err := s.QueryState(ctx)
	if err != nil {
		return nil, err
	}
	digest, err := stateDigest(state)
	if err != nil {
		return nil, fmt.Errorf("digest device state evidence: %w", err)
	}
	feedback := map[string]any{
		"source":         "device.query_state",
		"target":         currentOutputTarget(state),
		"observed_state": state["current_output"],
		"state_digest":   digest,
	}
	feedbackJSON, err := canonicaljson.Marshal(feedback)
	if err != nil {
		return nil, fmt.Errorf("canonicalize device feedback: %w", err)
	}
	feedbackHash := sha256.Sum256(feedbackJSON)
	evidence := map[string]any{
		"source":          "device.query_state",
		"evidence_type":   "device_state_feedback",
		"device_id":       stateString(state, "device_id"),
		"boot_id":         stateString(state, "boot_id"),
		"state":           state,
		"state_digest":    digest,
		"feedback":        feedback,
		"feedback_digest": "sha256:" + hex.EncodeToString(feedbackHash[:]),
	}
	withoutDigest := cloneDocument(evidence)
	bundleJSON, err := canonicaljson.Marshal(withoutDigest)
	if err != nil {
		return nil, fmt.Errorf("canonicalize device evidence: %w", err)
	}
	bundleHash := sha256.Sum256(bundleJSON)
	evidence["evidence_digest"] = "sha256:" + hex.EncodeToString(bundleHash[:])
	return evidence, nil
}

func currentOutputTarget(state map[string]any) string {
	output, _ := state["current_output"].(map[string]any)
	target, _ := output["target"].(string)
	return target
}

func setEvidenceTarget(evidence map[string]any, target string) error {
	evidence["target"] = target
	delete(evidence, "evidence_digest")
	bundleJSON, err := canonicaljson.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("canonicalize device evidence with target: %w", err)
	}
	bundleHash := sha256.Sum256(bundleJSON)
	evidence["evidence_digest"] = "sha256:" + hex.EncodeToString(bundleHash[:])
	return nil
}

func (s *DeviceSession) failStateRefresh(err error) (map[string]any, error) {
	if s.telemetry != nil {
		s.telemetry.ObserveDeviceFrameError()
	}
	s.opened = false
	return nil, err
}

// invalidateTransportLocked makes the current session unusable after a
// partial or invalid wire exchange. The caller holds s.mu; Close can still be
// called later to release claims and perform its normal cleanup.
func (s *DeviceSession) invalidateTransportLocked() {
	s.opened = false
	if s.transport != nil {
		_ = s.transport.Close()
	}
}

func (s *DeviceSession) requireReconciliation(ctx context.Context, reason string) error {
	wasRequired := s.reconciliationRequired
	s.stateQueryRequired = true
	s.reconciliationRequired = true
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reconciliationPersistTimeout)
	defer cancel()
	barrierErr := s.reconciliation.Require(persistCtx, s.deviceID, s.bootID, s.authorityEpoch, s.ownerInstance, reason)
	if isAuthorityFailure(barrierErr) {
		recoveryErr := s.reconciliation.RequireAfterAuthorityLoss(persistCtx, s.deviceID, s.bootID, s.authorityEpoch, s.ownerInstance, reason)
		if recoveryErr == nil {
			barrierErr = nil
		} else {
			barrierErr = errors.Join(barrierErr, recoveryErr)
		}
	}
	if barrierErr != nil {
		s.opened = false
		return fmt.Errorf("persist reconciliation barrier: %w", barrierErr)
	}
	if !wasRequired && s.telemetry != nil {
		s.telemetry.ObserveReconciliationBarrier()
	}
	return nil
}

func isAuthorityFailure(err error) bool {
	return errors.Is(err, storage.ErrRuntimeOwnerBusy) || errors.Is(err, storage.ErrEpochKilled) || errors.Is(err, storage.ErrEpochDraining)
}

func (s *DeviceSession) applyRefreshedState(ctx context.Context, state map[string]any) error {
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
	s.stateDigest, err = stateDigest(state)
	if err != nil {
		return fmt.Errorf("digest device state refresh: %w", err)
	}
	if err := s.bindRefreshedState(ctx, state); err != nil {
		return err
	}
	if !previousSafeState && s.safeState && s.telemetry != nil {
		s.telemetry.ObserveSafeStateEntry()
	}
	return nil
}

func (s *DeviceSession) bindRefreshedState(ctx context.Context, state map[string]any) error {
	if err := s.authority.AssertRuntime(ctx, s.authorityEpoch); err != nil {
		s.reconciliationRequired = true
		s.opened = false
		return fmt.Errorf("assert authority before binding refreshed state: %w", err)
	}
	wasRequired := s.reconciliationRequired
	required, err := s.reconciliation.BindState(ctx, state, s.authorityEpoch, s.ownerInstance)
	if err != nil {
		// A refresh is a durable safety transition. If its barrier write cannot
		// be proven, this session is no longer safe to use.
		s.reconciliationRequired = true
		s.opened = false
		return fmt.Errorf("bind refreshed device state: %w", err)
	}
	s.reconciliationRequired = required
	s.stateQueryRequired = false
	if !wasRequired && s.reconciliationRequired && s.telemetry != nil {
		s.telemetry.ObserveReconciliationBarrier()
	}
	return nil
}

// ResolveReconciliation records typed state/feedback evidence for the device
// barrier. Unknown command outcomes must already have gone through the
// dispatcher reconciliation path; the durable store refuses to clear while
// command ledgers remain unresolved. Manual review leaves the barrier open.
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
