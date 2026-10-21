package domain

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const baselineSchemaURN = "urn:agentic-stream:baseline-choice:"

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
		if message := baselineChoiceProblem(intent, field, choices[field]); message != "" {
			return &CompileError{Path: baselinePath(intent, phase) + "." + field, Message: message}
		}
	}
	return nil
}

func baselineChoiceProblem(intent Intent, field, value string) string {
	properties, _ := intent.ParameterSchema["properties"].(map[string]any)
	property, declared := properties[field].(map[string]any)
	if !declared {
		return fmt.Sprintf("parameter %q is not declared by parameterSchema", field)
	}
	if !slices.Contains(intent.ModelWritableFields, field) {
		return fmt.Sprintf("parameter %q is not in modelWritableFields; the decision validator accepts a chosen value only for a model-writable field", field)
	}
	enum, _ := property["enum"].([]any)
	if !slices.Contains(enum, any(value)) {
		return fmt.Sprintf("value %q is not in the enum of parameter %q", value, field)
	}
	return propertySchemaProblem(intent.Type, field, property, value)
}

func propertySchemaProblem(intentType, field string, property map[string]any, value string) string {
	document, err := json.Marshal(property)
	if err != nil {
		return fmt.Sprintf("parameter %q schema cannot be encoded: %v", field, err)
	}
	schema, err := canonicaljson.CompileSchemaJSON(baselineSchemaURN+intentType+"/"+field, document)
	if err != nil {
		return fmt.Sprintf("parameter %q schema does not compile: %v", field, err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Sprintf("value %q does not satisfy the schema of parameter %q: %v", value, field, err)
	}
	return ""
}

func baselinePath(intent Intent, phase string) string {
	return fmt.Sprintf("actions.intents.%s.baseline.byPhase.%s", intent.Type, phase)
}
