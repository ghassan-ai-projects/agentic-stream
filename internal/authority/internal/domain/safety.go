package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// SafeStopStage is one stage of a safe stop. Stages are recorded on the
// priority path and latch the device boot.
type SafeStopStage string

// Safe-stop stages.
const (
	SafeStopRequested SafeStopStage = "safe_stop_requested"
	SafeStopCompleted SafeStopStage = "safe_stop_completed"
	SafeStopFailed    SafeStopStage = "safe_stop_failed"
)

// SafeStopStages lists every stage; any recorded stage latches the boot.
var SafeStopStages = []SafeStopStage{SafeStopRequested, SafeStopCompleted, SafeStopFailed}

// Valid reports whether the stage is one of the safe-stop stages.
func (s SafeStopStage) Valid() bool {
	return s == SafeStopRequested || s == SafeStopCompleted || s == SafeStopFailed
}

// SafeStopEvent is the audit record of a safe-stop stage.
func SafeStopEvent(claim TargetClaim, stage SafeStopStage, details map[string]any, now time.Time) AuthorityEvent {
	return newEvent(EventType(stage), claim, details, now)
}

// SafetyEventType names one kind of safety evidence for the soak verdict.
type SafetyEventType string

// Safety event types: zero-tolerance violations and physical transitions.
const (
	SafetyUnsafeOutput                  SafetyEventType = "unsafe_output"
	SafetyStaleEnergizingEffect         SafetyEventType = "stale_energizing_effect"
	SafetyDuplicateNetEnergizingEffect  SafetyEventType = "duplicate_net_energizing_effect"
	SafetyUnexplainedActuatorTransition SafetyEventType = "unexplained_actuator_transition"
	SafetyFalseVerifiedSuccess          SafetyEventType = "false_verified_success"
	SafetySafeStateDeadlineMiss         SafetyEventType = "safe_state_deadline_miss"
	SafetyPhysicalTransition            SafetyEventType = "physical_transition"
)

func (t SafetyEventType) valid() bool {
	switch t {
	case SafetyUnsafeOutput, SafetyStaleEnergizingEffect, SafetyDuplicateNetEnergizingEffect,
		SafetyUnexplainedActuatorTransition, SafetyFalseVerifiedSuccess, SafetySafeStateDeadlineMiss, SafetyPhysicalTransition:
		return true
	default:
		return false
	}
}

// SafetyEvent is one piece of explicit safety evidence. Arbitrary telemetry
// labels cannot become safety claims.
type SafetyEvent struct {
	Type      SafetyEventType
	Target    string
	CommandID string
	Details   map[string]any
	Occurred  time.Time
}

// PrepareSafetyEvent validates the event and fills its defaults: no details
// becomes an empty document, and a zero time becomes now.
func PrepareSafetyEvent(event SafetyEvent, now time.Time) (SafetyEvent, error) {
	if event.Target == "" || !event.Type.valid() {
		return SafetyEvent{}, fmt.Errorf("safety event type and target are required")
	}
	if event.Occurred.IsZero() {
		event.Occurred = now
	}
	if event.Details == nil {
		event.Details = map[string]any{}
	}
	if event.Type == SafetyPhysicalTransition && claimsComplete(event.Details) && !PhysicalEvidenceComplete(event.Details) {
		return SafetyEvent{}, fmt.Errorf("complete physical transition evidence requires source and sha256 evidence_digest")
	}
	return event, nil
}

// PhysicalEvidenceComplete reports whether a physical transition marked
// complete names its source and a SHA-256 evidence digest. It checks the
// evidence's provenance shape, not the truth of the observation.
func PhysicalEvidenceComplete(details map[string]any) bool {
	if !claimsComplete(details) {
		return false
	}
	source, _ := details["source"].(string)
	digest, _ := details["evidence_digest"].(string)
	_, err := canonicaljson.DecodeDigest(digest)
	return source != "" && err == nil
}

func claimsComplete(details map[string]any) bool {
	complete, _ := details["evidence_complete"].(bool)
	return complete
}
