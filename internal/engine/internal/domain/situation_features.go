package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func SituationFeatures(compiled *spec.CompiledSpec, situationID string, stateJSON []byte) (map[string]any, error) {
	state, err := decodeState(situationID, stateJSON)
	if err != nil {
		return nil, err
	}
	if state.Evidence == nil {
		state.Evidence = []string{}
	}
	return situations.CELFeatures(compiled, state.Facts, state.Evidence), nil
}
