package operators

import (
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/operators/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Runtime applies compiled operators to normalized events.
type OperatorRuntime = domain.OperatorRuntime

// NewRuntime creates an operator runtime for the compiled spec.
func NewOperatorRuntime(deploymentID string, compiled *spec.CompiledSpec, idGen sources.Generator) (*domain.OperatorRuntime, error) {
	return domain.NewOperatorRuntime(deploymentID, compiled, idGen)
}

// OperatorStateBlob is the JSON-serializable state for one operator key.
type OperatorStateBlob = domain.OperatorStateBlob

// Feature is an emitted operator result.
type Feature = domain.Feature

// HeartbeatState stores the last seen heartbeat for a keyed entity.
type HeartbeatState = domain.HeartbeatState

// PartitionState is the in-memory operator state for one partition.
type PartitionState = domain.PartitionState

// Completeness describes how complete a feature or a Situation's evidence is.
type Completeness = domain.Completeness

// CompletenessCorrected marks a value recomputed with late evidence.
const CompletenessCorrected = domain.CompletenessCorrected

// CompletenessFinalByPolicy marks a closed window's value.
const CompletenessFinalByPolicy = domain.CompletenessFinalByPolicy

// WeakestCompleteness returns the least complete of values, or "" when there are none.
func WeakestCompleteness(values []Completeness) Completeness {
	return domain.Weakest(values)
}

// CompletenessProvisional marks an early value of a window that has not closed.
const CompletenessProvisional = domain.CompletenessProvisional

// CompletenessOnTime marks a value whose evidence arrived on time.
const CompletenessOnTime = domain.CompletenessOnTime

// CompletenessUncertain marks a value whose evidence may be missing.
const CompletenessUncertain = domain.CompletenessUncertain
