package situations

import (
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/situations/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// CELFeatures is the CEL `features` view of a Situation: its reduced facts
// and evidence, with every missing operator output defaulted so expressions
// never fail on a missing key. Situations and cognition share this one view.
func CELFeatures(compiled *spec.CompiledSpec, facts map[string]any, evidence []string) map[string]any {
	return domain.CELFeatures(compiled, facts, evidence)
}

// Engine evaluates features and maintains Situation state for one partition.
type Engine = domain.Engine

// Situation is the mutable current state for one occurrence.
type Situation = domain.Situation

// Version is an immutable Situation version.
type Version = domain.Version

// NewEngine creates a situation engine.
func NewEngine(deploymentID, tenantID string, partitionID int, compiled *spec.CompiledSpec, idGen sources.Generator) (*domain.Engine, error) {
	return domain.NewEngine(deploymentID, tenantID, partitionID, compiled, idGen)
}
