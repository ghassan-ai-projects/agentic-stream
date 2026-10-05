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
	// ReconciliationEvidence is digest-bound evidence about one device boot.
	ReconciliationEvidence = domain.ReconciliationEvidence
	// SafetyRecord summarizes the durable safety evidence a soak verdict reads.
	SafetyRecord = domain.SafetyRecord
)

// Safety event types: zero-tolerance violations and physical transitions.
const (
	SafetyUnsafeOutput                  = domain.SafetyUnsafeOutput
	SafetyStaleEnergizingEffect         = domain.SafetyStaleEnergizingEffect
	SafetyDuplicateNetEnergizingEffect  = domain.SafetyDuplicateNetEnergizingEffect
	SafetyUnexplainedActuatorTransition = domain.SafetyUnexplainedActuatorTransition
	SafetyFalseVerifiedSuccess          = domain.SafetyFalseVerifiedSuccess
	SafetySafeStateDeadlineMiss         = domain.SafetySafeStateDeadlineMiss
	SafetyPhysicalTransition            = domain.SafetyPhysicalTransition
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

// SealReconciliationEvidence builds evidence for the device boot named by
// state, with its state, feedback and bundle digests. Its Document is the wire
// form the authority later parses.
func SealReconciliationEvidence(source, target string, state, feedback map[string]any) (ReconciliationEvidence, error) {
	return domain.SealReconciliationEvidence(source, target, state, feedback)
}

// ParseReconciliationEvidence validates an evidence document for device and
// returns it typed.
func ParseReconciliationEvidence(document map[string]any, device DeviceBoot) (ReconciliationEvidence, error) {
	return domain.ParseReconciliationEvidence(document, device)
}

// PhysicalEvidenceComplete reports whether a physical transition marked
// complete names its source and a SHA-256 evidence digest.
func PhysicalEvidenceComplete(details map[string]any) bool {
	return domain.PhysicalEvidenceComplete(details)
}
