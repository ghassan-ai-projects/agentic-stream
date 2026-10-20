package domain

import (
	"fmt"
	"maps"
	"slices"
)

func checkIntentBaselines(intents []Intent, phases map[string]struct{}) error {
	for _, intent := range intents {
		if intent.Baseline == nil {
			continue
		}
		if err := checkIntentBaseline(intent, phases); err != nil {
			return err
		}
	}
	return nil
}

func checkIntentBaseline(intent Intent, phases map[string]struct{}) error {
	for _, phase := range slices.Sorted(maps.Keys(intent.Baseline.ByPhase)) {
		if _, declared := phases[phase]; !declared {
			return &CompileError{Path: baselinePath(intent, phase), Message: fmt.Sprintf("unknown phase %q", phase)}
		}
		if err := checkBaselineChoices(intent, phase); err != nil {
			return err
		}
	}
	return nil
}

func checkBaselineChoices(intent Intent, phase string) error {
	choices := intent.Baseline.ByPhase[phase]
	for _, field := range slices.Sorted(maps.Keys(choices)) {
		if message := baselineChoiceProblem(intent.ParameterSchema, field, choices[field]); message != "" {
			return &CompileError{Path: baselinePath(intent, phase) + "." + field, Message: message}
		}
	}
	return nil
}

func baselineChoiceProblem(parameterSchema map[string]any, field, value string) string {
	properties, _ := parameterSchema["properties"].(map[string]any)
	property, declared := properties[field].(map[string]any)
	if !declared {
		return fmt.Sprintf("parameter %q is not declared by parameterSchema", field)
	}
	enum, _ := property["enum"].([]any)
	if !slices.Contains(enum, any(value)) {
		return fmt.Sprintf("value %q is not in the enum of parameter %q", value, field)
	}
	return ""
}

func baselinePath(intent Intent, phase string) string {
	return fmt.Sprintf("actions.intents.%s.baseline.byPhase.%s", intent.Type, phase)
}
