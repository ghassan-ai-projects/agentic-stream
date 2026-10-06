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

const CompletenessProvisional = domain.CompletenessProvisional

const CompletenessOnTime = domain.CompletenessOnTime

const CompletenessUncertain = domain.CompletenessUncertain
