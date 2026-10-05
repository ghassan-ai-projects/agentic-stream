package replay

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// DeterministicBaseline is the in-repository non-model shadow policy. It
// selects only from the spec's declared intent catalog and emits no action
// plane object.
type DeterministicBaseline = domain.BaselinePolicy

// NewDeterministicBaseline creates the baseline from an immutable compiled
// spec. The caller still validates its output through the normal Decision and
// Intent catalog validator.
func NewDeterministicBaseline(compiled *spec.CompiledSpec) (*DeterministicBaseline, error) {
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
