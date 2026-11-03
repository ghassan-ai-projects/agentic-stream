package domain

import (
	"fmt"
	"slices"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/ast"
)

const featuresVariable = "features"

func ruleFeatureReads(source string, features []string, path string) ([]string, error) {
	checked, err := compileRuleCondition(source)
	if err != nil {
		return nil, &CompileError{Path: path, Message: err.Error()}
	}
	reads, err := featureReads(checked.NativeRep())
	if err != nil {
		return nil, &CompileError{Path: path, Message: err.Error()}
	}
	for _, name := range reads {
		if !slices.Contains(features, name) {
			return nil, &CompileError{Path: path, Message: fmt.Sprintf("feature %q is not produced by an operator", name)}
		}
	}
	return reads, nil
}

func compileRuleCondition(source string) (*cel.Ast, error) {
	env, err := NewCELEnv()
	if err != nil {
		return nil, err
	}
	checked, issues := env.Compile(source)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("cel: %w", issues.Err())
	}
	if checked.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("cel: a rule condition must be boolean, not %s", checked.OutputType())
	}
	return checked, nil
}

func featureReads(checked *ast.AST) ([]string, error) {
	var reads []string
	for _, ident := range ast.MatchDescendants(ast.NavigateAST(checked), ast.KindMatcher(ast.IdentKind)) {
		name, err := featureSelected(ident)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(reads, name) {
			reads = append(reads, name)
		}
	}
	slices.Sort(reads)
	return reads, nil
}

func featureSelected(ident ast.NavigableExpr) (string, error) {
	if ident.AsIdent() != featuresVariable {
		return "", fmt.Errorf("a rule condition may read only %s, not %s", featuresVariable, ident.AsIdent())
	}
	parent, ok := ident.Parent()
	if !ok || parent.Kind() != ast.SelectKind {
		return "", fmt.Errorf("a rule condition must name each feature it reads, as %s.<name>", featuresVariable)
	}
	return parent.AsSelect().FieldName(), nil
}

func (s *CompiledSpec) ConditionReads(when string) ([]string, error) {
	return ruleFeatureReads(when, s.FeatureNames(), "when")
}
