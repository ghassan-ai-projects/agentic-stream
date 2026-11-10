package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func (e *Engine) recordEvidenceQuality(sit *Situation, feature operators.Feature) bool {
	if input, advisory := e.spec.AdvisoryHeartbeatInput(feature.OutputName); advisory {
		return recordInputStaleness(sit, input, feature.Value == true)
	}
	return recordInputCompleteness(sit, feature)
}

func recordInputStaleness(sit *Situation, input string, stale bool) bool {
	if sit.Facts == nil {
		sit.Facts = make(map[string]any)
	}
	fact := spec.InputStaleFact(input)
	was, _ := sit.Facts[fact].(bool)
	sit.Facts[fact] = stale
	return was != stale
}

func addInputStaleness(features map[string]any, compiled *spec.CompiledSpec, facts map[string]any) {
	advisory := compiled.AdvisoryInputs()
	if len(advisory) == 0 {
		return
	}
	stale := make(map[string]any, len(advisory))
	for _, input := range advisory {
		lost, _ := facts[spec.InputStaleFact(input)].(bool)
		stale[input] = lost
	}
	features[spec.InputStaleFacts] = stale
}
