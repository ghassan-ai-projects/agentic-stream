package domain

import (
	"fmt"
	"slices"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type RuleKey struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type Rules struct {
	spec     *spec.CompiledSpec
	book     *spec.CompiledPlaybook
	digest   string
	context  []string
	compiled []compiledRule
}

type compiledRule struct {
	rule    spec.Rule
	program cel.Program
}

func NewRules(compiled *spec.CompiledSpec, playbook *spec.CompiledPlaybook) (*Rules, error) {
	rules := &Rules{spec: compiled, context: contextFeatures(compiled)}
	if playbook == nil {
		return rules, nil
	}
	rules.digest, rules.book = playbook.Digest, playbook
	var err error
	rules.compiled, err = compileRules(playbook.Rules)
	return rules, err
}

func compileRules(rules []spec.Rule) ([]compiledRule, error) {
	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		c, err := compileRule(rule)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, c)
	}
	return compiled, nil
}

func compileRule(rule spec.Rule) (compiledRule, error) {
	env, err := spec.NewCELEnv()
	if err != nil {
		return compiledRule{}, fmt.Errorf("create rule condition environment: %w", err)
	}
	program, err := conditionProgram(env, rule)
	if err != nil {
		return compiledRule{}, err
	}
	return compiledRule{rule: rule, program: program}, nil
}

func contextFeatures(compiled *spec.CompiledSpec) []string {
	return slices.Clone(compiled.Promotion.ContextFeatureNames())
}

func conditionProgram(env *cel.Env, rule spec.Rule) (cel.Program, error) {
	checked, issues := env.Compile(rule.When)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile the condition of rule %s: %w", rule.ID, issues.Err())
	}
	program, err := env.Program(checked)
	if err != nil {
		return nil, fmt.Errorf("plan the condition of rule %s: %w", rule.ID, err)
	}
	return program, nil
}

func (r *Rules) Digest() string { return r.digest }

func (r *Rules) Features(facts map[string]any, evidence []string) map[string]any {
	return situations.CELFeatures(r.spec, facts, evidence)
}

func KeyOf(rule spec.Rule) RuleKey {
	return RuleKey{ID: rule.ID, Version: rule.Version}
}

func (c compiledRule) matches(features map[string]any) bool {
	out, _, err := c.program.Eval(map[string]any{"features": features})
	if err != nil {
		return false
	}
	matched, err := spec.CELBool(out)
	return err == nil && matched
}

func (r *Rules) Spec() *spec.CompiledSpec { return r.spec }

func (r *Rules) WithRule(rule spec.Rule) *spec.CompiledPlaybook {
	book := &spec.CompiledPlaybook{SchemaVersion: "agentic-stream/v1", Metadata: spec.PlaybookMetadata{Name: "playbook", Spec: r.spec.Metadata.Name}}
	if r.book != nil {
		book.Metadata, book.Rules = r.book.Metadata, slices.Clone(r.book.Rules)
	}
	book.PutRule(rule)
	return book
}

func (r *Rules) NextVersion(ruleID string) int {
	version := 0
	for _, compiled := range r.compiled {
		if compiled.rule.ID == ruleID {
			version = max(version, compiled.rule.Version)
		}
	}
	return version + 1
}

func (r *Rules) ShadowRules() []spec.Rule {
	var shadow []spec.Rule
	for _, compiled := range r.compiled {
		if compiled.rule.Mode == spec.RuleModeShadow {
			shadow = append(shadow, compiled.rule)
		}
	}
	return shadow
}

func ExitMet(rule spec.Rule, agree, disagree int) bool {
	return rule.ShadowExit != nil && agree >= rule.ShadowExit.Agreeing && disagree <= rule.ShadowExit.Disagreeing
}
