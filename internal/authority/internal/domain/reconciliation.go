package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ErrReconciliationRequired means a reconciliation is open for the device, so
// ordinary commands are blocked.
var ErrReconciliationRequired = errors.New("device reconciliation is required")

// ErrNoOpenReconciliation means a resolution was attempted while the device
// boot has no open reconciliation.
var ErrNoOpenReconciliation = errors.New("device boot has no open reconciliation")

// ErrNoRecordedState means the device has never reported its state.
var ErrNoRecordedState = errors.New("device has no recorded state")

// ReconciliationStatus is clear, or required while a reconciliation is open.
type ReconciliationStatus string

// Reconciliation statuses.
const (
	ReconciliationClear    ReconciliationStatus = "clear"
	ReconciliationRequired ReconciliationStatus = "required"
)

// Reconciliation is the recorded reconciliation state of one device: its
// current boot, status, and the digest of its latest reported state.
type Reconciliation struct {
	Device      DeviceBoot
	Status      ReconciliationStatus
	StateSHA256 []byte
}

// Required reports whether a reconciliation is open.
func (r *Reconciliation) Required() bool {
	return r != nil && r.Status == ReconciliationRequired
}

// ReconciliationOpening asks to open a reconciliation for a device boot.
type ReconciliationOpening struct {
	Device DeviceBoot
	Owner  Owner
	Reason string
}

// Complete reports whether the opening names its device boot, owner and reason.
func (o ReconciliationOpening) Complete() bool {
	return o.Device.Complete() && o.Owner.Complete() && o.Reason != ""
}

// CheckOpenable requires a recorded state for the same boot. It reports
// whether a reconciliation is already open, in which case only the reason is
// audited again.
func CheckOpenable(recorded *Reconciliation, device DeviceBoot) (bool, error) {
	if recorded == nil {
		return false, fmt.Errorf("open reconciliation for %q: %w", device.DeviceID, ErrNoRecordedState)
	}
	if recorded.Device.BootID != device.BootID {
		return false, fmt.Errorf("reconciliation boot %q does not match current device boot %q", device.BootID, recorded.Device.BootID)
	}
	return recorded.Required(), nil
}

// OpeningEvent is the audit record of an opened reconciliation.
func OpeningEvent(opening ReconciliationOpening, now time.Time) AuthorityEvent {
	return newEvent(EventReconciliationOpened, DeviceSubject(opening.Device, opening.Owner), map[string]any{"reason": opening.Reason}, now)
}

// ResolutionOutcome is the result recorded when a reconciliation is resolved.
type ResolutionOutcome string

// Resolution outcomes. Succeeded and failed clear the reconciliation; manual
// review keeps it required.
const (
	ResolutionSucceeded    ResolutionOutcome = "succeeded"
	ResolutionFailed       ResolutionOutcome = "failed"
	ResolutionManualReview ResolutionOutcome = "manual_review"
)

// Valid reports whether the outcome is one of the three resolution outcomes.
func (o ResolutionOutcome) Valid() bool {
	return o == ResolutionSucceeded || o == ResolutionFailed || o == ResolutionManualReview
}

// Clears reports whether the outcome closes the reconciliation.
func (o ResolutionOutcome) Clears() bool {
	return o != ResolutionManualReview
}

// StatusAfter is the reconciliation status once the outcome is recorded.
func (o ResolutionOutcome) StatusAfter() ReconciliationStatus {
	if o.Clears() {
		return ReconciliationClear
	}
	return ReconciliationRequired
}

// ResolutionRequest asks to resolve the open reconciliation of a device boot
// with an outcome and an evidence document.
type ResolutionRequest struct {
	Device   DeviceBoot
	Owner    Owner
	Outcome  ResolutionOutcome
	Evidence map[string]any
}

// Check requires a known outcome, a device boot and an evidence document.
func (r ResolutionRequest) Check() error {
	if !r.Outcome.Valid() {
		return fmt.Errorf("invalid resolution outcome %q", r.Outcome)
	}
	if !r.Device.Complete() || len(r.Evidence) == 0 {
		return errors.New("device boot and reconciliation evidence are required")
	}
	return nil
}

// Resolution is a validated resolution ready to be recorded.
type Resolution struct {
	Device         DeviceBoot
	Owner          Owner
	Outcome        ResolutionOutcome
	Evidence       ReconciliationEvidence
	EvidenceJSON   []byte
	EvidenceSHA256 []byte
}

// NewResolution parses the request's evidence for its device boot and keeps
// the document's canonical form, which is what gets stored.
func NewResolution(request ResolutionRequest) (Resolution, error) {
	evidence, err := ParseReconciliationEvidence(request.Evidence, request.Device)
	if err != nil {
		return Resolution{}, err
	}
	evidenceJSON, err := canonicaljson.Marshal(request.Evidence)
	if err != nil {
		return Resolution{}, fmt.Errorf("canonicalize reconciliation evidence: %w", err)
	}
	return Resolution{Device: request.Device, Owner: request.Owner, Outcome: request.Outcome, Evidence: evidence, EvidenceJSON: evidenceJSON, EvidenceSHA256: canonicaljson.Sum(evidenceJSON)}, nil
}

// CheckResolvable requires an open reconciliation for the resolution's boot,
// and evidence whose typed state matches both its own digest and the latest
// reported state.
func CheckResolvable(recorded *Reconciliation, resolution Resolution) error {
	if recorded == nil {
		return fmt.Errorf("resolve reconciliation for %q: %w", resolution.Device.DeviceID, ErrNoRecordedState)
	}
	if !recorded.Required() || recorded.Device.BootID != resolution.Device.BootID {
		return ErrNoOpenReconciliation
	}
	return checkLatestStateEvidence(resolution.Evidence, recorded.StateSHA256)
}

func checkLatestStateEvidence(evidence ReconciliationEvidence, latestStateSHA256 []byte) error {
	if evidence.StateDigest != canonicaljson.EncodeDigest(latestStateSHA256) {
		return fmt.Errorf("reconciliation evidence does not bind the latest device state")
	}
	stateDigest, err := documentDigest("reconciliation state evidence", evidence.State)
	if err != nil {
		return err
	}
	if evidence.StateDigest != stateDigest {
		return fmt.Errorf("reconciliation state digest does not match typed state evidence")
	}
	return nil
}

// CheckCommandsReconciled refuses a resolution while commands bound to the
// device boot still await reconciliation in the action ledger.
func CheckCommandsReconciled(unresolved int64) error {
	if unresolved > 0 {
		return fmt.Errorf("cannot clear device reconciliation while %d command outcomes still require dispatcher reconciliation", unresolved)
	}
	return nil
}

// ResolutionEvent is the audit record of a recorded resolution.
func ResolutionEvent(resolution Resolution, now time.Time) AuthorityEvent {
	return newEvent(EventReconciliationRecorded, DeviceSubject(resolution.Device, resolution.Owner), map[string]any{
		"final_status":    string(resolution.Outcome),
		"evidence_sha256": canonicaljson.EncodeDigest(resolution.EvidenceSHA256),
		"barrier_cleared": resolution.Outcome.Clears(),
	}, now)
}
