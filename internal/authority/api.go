package authority

import "github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"

// Value types callers pass to the Service. They are aliases of the domain
// vocabulary; the domain's internal model is not exported.
type (
	// Owner is the runtime process that holds the singleton runtime lease.
	Owner = domain.Owner
	// DeviceBoot is one power-on of one device.
	DeviceBoot = domain.DeviceBoot
	// TargetClaim is an owner's right to command one target on one device boot.
	TargetClaim = domain.TargetClaim
	// CommandBinding ties a command to the target, device boot and owner that delivered it.
	CommandBinding = domain.CommandBinding
	// ResolutionOutcome is the result recorded when a reconciliation is resolved.
	ResolutionOutcome = domain.ResolutionOutcome
	// SafeStopStage is one stage of a safe stop.
	SafeStopStage = domain.SafeStopStage
	// SafetyEvent is one piece of explicit safety evidence for the soak verdict.
	SafetyEvent = domain.SafetyEvent
	// SafetyEventType names one kind of safety evidence.
	SafetyEventType = domain.SafetyEventType
)

// Resolution outcomes.
const (
	ResolutionSucceeded    = domain.ResolutionSucceeded
	ResolutionFailed       = domain.ResolutionFailed
	ResolutionManualReview = domain.ResolutionManualReview
)

// Safe-stop stages.
const (
	SafeStopRequested = domain.SafeStopRequested
	SafeStopCompleted = domain.SafeStopCompleted
	SafeStopFailed    = domain.SafeStopFailed
)

// Sentinel errors callers branch on.
var (
	// ErrTargetClaimBusy means another owner holds a live claim on the target.
	ErrTargetClaimBusy = domain.ErrTargetClaimBusy
	// ErrTargetClaimNotOwned means the caller is not the exact live holder of the claim.
	ErrTargetClaimNotOwned = domain.ErrTargetClaimNotOwned
	// ErrReconciliationRequired means ordinary commands are blocked until the device is reconciled.
	ErrReconciliationRequired = domain.ErrReconciliationRequired
	// ErrNoOpenReconciliation means a resolution was attempted with no open reconciliation.
	ErrNoOpenReconciliation = domain.ErrNoOpenReconciliation
	// ErrNoRecordedState means the device has never reported its state.
	ErrNoRecordedState = domain.ErrNoRecordedState
)

// ValidateReconciliationEvidence validates a typed, boot-bound reconciliation
// evidence envelope for device. It is a stateless rule other modules apply to
// evidence before they persist it.
func ValidateReconciliationEvidence(evidence map[string]any, device DeviceBoot) error {
	return domain.ValidateReconciliationEvidence(evidence, device)
}

// PhysicalEvidenceComplete reports whether a physical transition marked
// complete names its source and a SHA-256 evidence digest.
func PhysicalEvidenceComplete(details map[string]any) bool {
	return domain.PhysicalEvidenceComplete(details)
}
