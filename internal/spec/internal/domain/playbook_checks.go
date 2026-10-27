package domain

import (
	"fmt"
	"maps"
	"slices"
)

func checkPlaybook(spec *CompiledSpec, playbook *CompiledPlaybook) error {
	if playbook.Metadata.Spec != spec.Metadata.Name {
		return &CompileError{Path: "metadata.spec", Message: fmt.Sprintf("playbook targets spec %q, not %q", playbook.Metadata.Spec, spec.Metadata.Name)}
	}
	codes, err := catalogForRules(spec, len(playbook.Rules))
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(playbook.Rules))
	for i := range playbook.Rules {
		if err := checkRule(spec, codes, &playbook.Rules[i], fmt.Sprintf("rules[%d]", i), seen); err != nil {
			return err
		}
	}
	return nil
}

func catalogForRules(spec *CompiledSpec, rules int) ([]string, error) {
	codes, err := spec.DiagnosisCodes()
	if err != nil {
		return nil, &CompileError{Path: "spec.cognition.executor.diagnosisCatalog", Message: err.Error()}
	}
	if rules > 0 && len(codes) == 0 {
		return nil, &CompileError{Path: "rules", Message: "rules need a spec that declares a diagnosis catalog"}
	}
	return codes, nil
}

func checkRule(spec *CompiledSpec, codes []string, rule *Rule, path string, seen map[string]struct{}) error {
	if err := admitRuleID(rule.ID, path+".id", seen); err != nil {
		return err
	}
	if err := checkRuleDiagnosis(spec, codes, rule, path); err != nil {
		return err
	}
	if err := checkRuleRecommendation(spec, rule.Recommend, path+".recommend"); err != nil {
		return err
	}
	reads, err := ruleFeatureReads(rule.When, spec.FeatureNames(), path+".when")
	if err != nil {
		return err
	}
	rule.Reads = reads
	return checkRuleRanges(rule, contextFeatures(spec), path)
}

func admitRuleID(id, path string, seen map[string]struct{}) error {
	if _, duplicate := seen[id]; duplicate {
		return &CompileError{Path: path, Message: fmt.Sprintf("rule %q is declared twice", id)}
	}
	seen[id] = struct{}{}
	return nil
}

func checkRuleDiagnosis(spec *CompiledSpec, codes []string, rule *Rule, path string) error {
	if rule.DiagnosisCode == UnknownDiagnosis {
		return &CompileError{Path: path + ".diagnosisCode", Message: "the unknown diagnosis never becomes a rule"}
	}
	if !slices.Contains(codes, rule.DiagnosisCode) {
		return &CompileError{Path: path + ".diagnosisCode", Message: fmt.Sprintf("diagnosis code %q is not in the diagnosis catalog", rule.DiagnosisCode)}
	}
	if remedies, mapped := remedyIntents(spec, rule.DiagnosisCode); mapped && !slices.Contains(remedies, rule.Recommend.Intent) {
		return &CompileError{Path: path + ".recommend.intent", Message: fmt.Sprintf("the remedy map does not allow %q for %q", rule.Recommend.Intent, rule.DiagnosisCode)}
	}
	return nil
}

func remedyIntents(spec *CompiledSpec, diagnosisCode string) ([]string, bool) {
	if spec.Promotion == nil {
		return nil, false
	}
	remedies, mapped := spec.Promotion.RemedyMap[diagnosisCode]
	return remedies, mapped
}

func checkRuleRecommendation(spec *CompiledSpec, recommend Recommendation, path string) error {
	intent, declared := spec.Intent(recommend.Intent)
	if !declared {
		return &CompileError{Path: path + ".intent", Message: fmt.Sprintf("intent %q is not declared", recommend.Intent)}
	}
	if _, ok := intent.Presets[recommend.Preset]; !ok {
		return &CompileError{Path: path + ".preset", Message: fmt.Sprintf("intent %q has no preset %q", recommend.Intent, recommend.Preset)}
	}
	return checkRuleTarget(intent, path+".intent")
}

func checkRuleTarget(intent Intent, path string) error {
	switch {
	case intent.Risk >= "R3":
		return &CompileError{Path: path, Message: fmt.Sprintf("a rule may not recommend %q: risk %s is R3 or above", intent.Type, intent.Risk)}
	case intent.Emergency:
		return &CompileError{Path: path, Message: fmt.Sprintf("a rule may not recommend %q: it is an emergency intent", intent.Type)}
	case intent.SafetyFunction:
		return &CompileError{Path: path, Message: fmt.Sprintf("a rule may not recommend %q: it is part of a safety function", intent.Type)}
	}
	return nil
}

func contextFeatures(spec *CompiledSpec) []string {
	if spec.Promotion == nil {
		return nil
	}
	return spec.Promotion.ContextFeatures
}

func checkRuleRanges(rule *Rule, context []string, path string) error {
	allowed := append(slices.Clone(rule.Reads), context...)
	for _, feature := range slices.Sorted(maps.Keys(rule.Cap)) {
		if err := checkCapRange(rule.Cap[feature], allowed, feature, path+".cap."+feature); err != nil {
			return err
		}
	}
	for _, feature := range slices.Sorted(maps.Keys(rule.Envelope)) {
		if err := checkEnvelopeRange(rule, feature, path+".envelope."+feature); err != nil {
			return err
		}
	}
	return nil
}

func checkCapRange(limit Range, allowed []string, feature, path string) error {
	if !slices.Contains(allowed, feature) {
		return &CompileError{Path: path, Message: "a range may only cover features the rule reads or declared context features"}
	}
	if limit.Min > limit.Max {
		return &CompileError{Path: path, Message: "min is above max"}
	}
	return nil
}

func checkEnvelopeRange(rule *Rule, feature, path string) error {
	envelope := rule.Envelope[feature]
	limit, capped := rule.Cap[feature]
	if !capped {
		return &CompileError{Path: path, Message: "every envelope feature needs a cap"}
	}
	if envelope.Min > envelope.Max || !envelope.Within(limit) {
		return &CompileError{Path: path, Message: "the envelope must be a range inside its cap"}
	}
	return nil
}
