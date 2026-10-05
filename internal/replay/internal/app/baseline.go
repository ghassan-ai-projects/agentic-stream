package app

import (
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// NewDeterministicBaseline creates the default baseline executor from an
// immutable compiled spec by projecting its intent catalog into domain
// intents; validation of its output stays with the shadow rules.
func NewDeterministicBaseline(compiled *spec.CompiledSpec) (*domain.BaselinePolicy, error) {
	return domain.NewBaselinePolicy(baselineIntents(compiled))
}

func baselineIntents(compiled *spec.CompiledSpec) []domain.Intent {
	if compiled == nil {
		return nil
	}
	intents := make([]domain.Intent, 0, len(compiled.Actions.Intents))
	for _, configured := range compiled.Actions.Intents {
		intents = append(intents, domain.Intent{Type: configured.Type, Risk: configured.Risk, ParameterSchema: configured.ParameterSchema})
	}
	return intents
}
