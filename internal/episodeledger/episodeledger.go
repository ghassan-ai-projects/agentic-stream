package episodeledger

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// Episode and attempt vocabulary. The domain owns the values; the facade
// re-exports them so the modules that consume the ledger share one language.
type (
	// LifecycleStatus is the coordination state of an episode aggregate.
	LifecycleStatus = domain.LifecycleStatus
	// AttemptStatus is the state of one worker dispatch.
	AttemptStatus = domain.AttemptStatus
	// RejectionReason is a durable reason for refusing worker input.
	RejectionReason = domain.RejectionReason
	// Identity is the fencing identity carried by every worker-produced object.
	Identity = domain.Identity
	// IdentityError identifies why worker input was refused.
	IdentityError = domain.IdentityError
	// Admission is the materialized durable episode input.
	Admission = domain.Admission
	// SchedulerItem is one durable scheduler entry.
	SchedulerItem = domain.SchedulerItem
	// SchedulingRecord is what became of one admitted trigger evaluation.
	SchedulingRecord = domain.SchedulingRecord
	// RejectionRecord is one worker result the ledger refused, with its reason.
	RejectionRecord = domain.RejectionRecord
	// RecoveryReport describes active attempt state abandoned during a restart.
	RecoveryReport = domain.RecoveryReport
	// CostSettler settles or releases an episode's cost reservation on the
	// caller's transaction; the runtime's cost ledger satisfies it.
	CostSettler = store.Settler
)

// ErrLiveEpisodeConflict means a reconsideration collides with a live episode.
var ErrLiveEpisodeConflict = domain.ErrLiveEpisodeConflict

// Episode lifecycle states.
const (
	LifecycleAdmitted   = domain.LifecycleAdmitted
	LifecycleRunning    = domain.LifecycleRunning
	LifecycleConcluded  = domain.LifecycleConcluded
	LifecycleClosed     = domain.LifecycleClosed
	LifecycleSuperseded = domain.LifecycleSuperseded
	LifecycleExpired    = domain.LifecycleExpired
	LifecycleAbandoned  = domain.LifecycleAbandoned
)

// Attempt states.
const (
	AttemptDispatched = domain.AttemptDispatched
	AttemptRunning    = domain.AttemptRunning
	AttemptCancelling = domain.AttemptCancelling //nolint:misspell // Frozen durable protocol value.
	AttemptProduced   = domain.AttemptProduced
	AttemptDeclined   = domain.AttemptDeclined
	AttemptCancelled  = domain.AttemptCancelled //nolint:misspell // Frozen durable protocol value.
	AttemptFailed     = domain.AttemptFailed
	AttemptTimedOut   = domain.AttemptTimedOut
	AttemptAbandoned  = domain.AttemptAbandoned
)

// Rejection reasons.
const (
	RejectUnknownEpisode          = domain.RejectUnknownEpisode
	RejectStaleAttempt            = domain.RejectStaleAttempt
	RejectWrongAttempt            = domain.RejectWrongAttempt
	RejectTerminalAttempt         = domain.RejectTerminalAttempt
	RejectEpisodeClosed           = domain.RejectEpisodeClosed
	RejectSchemaInvalid           = domain.RejectSchemaInvalid
	RejectSnapshotMismatch        = domain.RejectSnapshotMismatch
	RejectEvidenceNotVisible      = domain.RejectEvidenceNotVisible
	RejectForgedReference         = domain.RejectForgedReference
	RejectOversized               = domain.RejectOversized
	RejectExpired                 = domain.RejectExpired
	RejectIntentTypeNotAllowed    = domain.RejectIntentTypeNotAllowed
	RejectRiskCeilingExceeded     = domain.RejectRiskCeilingExceeded
	RejectCatalogMissing          = domain.RejectCatalogMissing
	RejectCatalogForged           = domain.RejectCatalogForged
	RejectIntentTypeNotInCatalog  = domain.RejectIntentTypeNotInCatalog
	RejectRiskLabelMismatch       = domain.RejectRiskLabelMismatch
	RejectParameterSchemaViolated = domain.RejectParameterSchemaViolated
	RejectPresetMismatch          = domain.RejectPresetMismatch
	RejectUngroundedEvidence      = domain.RejectUngroundedEvidence
)

// IsTerminalAttempt reports whether an attempt state is terminal.
func IsTerminalAttempt(status AttemptStatus) bool { return domain.IsTerminalAttempt(status) }
