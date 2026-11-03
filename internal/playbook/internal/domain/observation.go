package domain

import (
	"bytes"
	"maps"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type Observed struct {
	DiagnosisCode string `json:"diagnosis_code"`
	Intent        string `json:"intent"`
	Preset        string `json:"preset"`
}

func (r *Rules) Observe(decision map[string]any) Observed {
	observed := Observed{DiagnosisCode: contractsv1.DocumentString(decision, "diagnosis_code")}
	intent, found := actionableIntent(decision)
	if !found {
		return observed
	}
	observed.Intent = contractsv1.DocumentString(intent, "type")
	declared, _ := r.spec.Intent(observed.Intent)
	parameters, _ := intent["parameters"].(map[string]any)
	observed.Preset = PresetOf(declared, parameters)
	return observed
}

func actionableIntent(decision map[string]any) (map[string]any, bool) {
	intents, _ := decision["intents"].([]any)
	for _, raw := range intents {
		intent, _ := raw.(map[string]any)
		if intent != nil && contractsv1.DocumentString(intent, "type") != actionport.WatchInstallRoute &&
			contractsv1.DocumentString(intent, "compensates") == "" {
			return intent, true
		}
	}
	return nil, false
}

func PresetOf(intent spec.Intent, parameters map[string]any) string {
	for _, name := range slices.Sorted(maps.Keys(intent.Presets)) {
		if presetHolds(intent.Presets[name], parameters) {
			return name
		}
	}
	return ""
}

func presetHolds(preset, parameters map[string]any) bool {
	for key, want := range preset {
		if !sameValue(want, parameters[key]) {
			return false
		}
	}
	return len(preset) > 0
}

func sameValue(left, right any) bool {
	leftJSON, leftErr := canonicaljson.Marshal(left)
	rightJSON, rightErr := canonicaljson.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}
