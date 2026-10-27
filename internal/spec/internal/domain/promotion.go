package domain

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

type Promotion struct {
	SettleWindows   map[string]string   `json:"settleWindows,omitempty" yaml:"settleWindows,omitempty"`
	ContextFeatures []string            `json:"contextFeatures,omitempty" yaml:"contextFeatures,omitempty"`
	RemedyMap       map[string][]string `json:"remedyMap,omitempty" yaml:"remedyMap,omitempty"`
	EnvelopeMargin  float64             `json:"envelopeMargin,omitempty" yaml:"envelopeMargin,omitempty"`
}

const UnknownDiagnosis = "unknown"

func (s *CompiledSpec) DiagnosisCodes() ([]string, error) {
	return ParseDiagnosisCodes(s.Cognition.Executor.DiagnosisCatalog)
}

func ParseDiagnosisCodes(source string) ([]string, error) {
	if source == "" {
		return nil, nil
	}
	var entries []struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(source), &entries); err != nil {
		return nil, fmt.Errorf("decode diagnosis catalog: %w", err)
	}
	codes := make([]string, 0, len(entries))
	for _, entry := range entries {
		codes = append(codes, entry.Code)
	}
	return codes, nil
}

func (s *CompiledSpec) FeatureNames() []string {
	names := make([]string, 0, len(s.Operators)+len(s.Situation.Reducers))
	for _, op := range s.Operators {
		names = append(names, op.Output)
	}
	for _, reducer := range s.Situation.Reducers {
		if reducer.Strategy == "set_union" {
			names = append(names, reducer.Field)
		}
	}
	return names
}

func (s *CompiledSpec) Intent(intentType string) (Intent, bool) {
	for _, intent := range s.Actions.Intents {
		if intent.Type == intentType {
			return intent, true
		}
	}
	return Intent{}, false
}

func checkPromotion(spec *CompiledSpec) error {
	if spec.Promotion == nil {
		return nil
	}
	codes, err := spec.DiagnosisCodes()
	if err != nil {
		return &CompileError{Path: "cognition.executor.diagnosisCatalog", Message: err.Error()}
	}
	if err := checkSettleWindows(spec.Promotion.SettleWindows, codes); err != nil {
		return err
	}
	if err := checkRemedyMap(spec, codes); err != nil {
		return err
	}
	return checkContextFeatures(spec.Promotion.ContextFeatures, spec.FeatureNames())
}

func checkSettleWindows(windows map[string]string, codes []string) error {
	for _, code := range slices.Sorted(maps.Keys(windows)) {
		source := windows[code]
		path := "promotion.settleWindows." + code
		if !slices.Contains(codes, code) {
			return &CompileError{Path: path, Message: "diagnosis code is not in the diagnosis catalog"}
		}
		if _, err := ParseDuration(source); err != nil {
			return &CompileError{Path: path, Message: err.Error()}
		}
	}
	return nil
}

func checkRemedyMap(spec *CompiledSpec, codes []string) error {
	for _, code := range slices.Sorted(maps.Keys(spec.Promotion.RemedyMap)) {
		intents := spec.Promotion.RemedyMap[code]
		path := "promotion.remedyMap." + code
		if !slices.Contains(codes, code) {
			return &CompileError{Path: path, Message: "diagnosis code is not in the diagnosis catalog"}
		}
		for _, intentType := range intents {
			if _, ok := spec.Intent(intentType); !ok {
				return &CompileError{Path: path, Message: fmt.Sprintf("intent %q is not declared", intentType)}
			}
		}
	}
	return nil
}

func checkContextFeatures(context, features []string) error {
	for i, name := range context {
		if !slices.Contains(features, name) {
			return &CompileError{Path: fmt.Sprintf("promotion.contextFeatures[%d]", i), Message: fmt.Sprintf("feature %q is not produced by an operator", name)}
		}
	}
	return nil
}
