package domain

import (
	"fmt"
	"slices"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type RuleKey struct {
	ID      string
	Version int
}

type Rules struct {
	spec     *spec.CompiledSpec
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
	rules.digest = playbook.Digest
	var err error
	rules.compiled, err = compileRules(playbook.Rules)
	return rules, err
}

func compileRules(rules []spec.Rule) ([]compiledRule, error) {
	env, err := spec.NewCELEnv()
	if err != nil {
		return nil, fmt.Errorf("create rule condition environment: %w", err)
	}
	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		program, err := conditionProgram(env, rule)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, compiledRule{rule: rule, program: program})
	}
	return compiled, nil
}

func contextFeatures(compiled *spec.CompiledSpec) []string {
	if compiled.Promotion == nil {
		return nil
	}
	return slices.Clone(compiled.Promotion.ContextFeatures)
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

func keyOf(rule spec.Rule) RuleKey {
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
