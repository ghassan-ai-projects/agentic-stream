package domain

import (
	"fmt"
	"slices"
)

const (
	CompletenessRoleRequired = "required"
	CompletenessRoleAdvisory = "advisory"
	InputStaleFacts          = "input_stale"
)

func (in Input) Advisory() bool { return in.CompletenessRole == CompletenessRoleAdvisory }

func (s *CompiledSpec) AdvisoryInputs() []string {
	var names []string
	for _, in := range s.Inputs {
		if in.Advisory() {
			names = append(names, in.Name)
		}
	}
	return names
}

func (s *CompiledSpec) AdvisoryHeartbeatInput(operatorOutput string) (string, bool) {
	for _, op := range s.Operators {
		if op.Kind == "missing_heartbeat" && op.Output == operatorOutput && len(op.Inputs) == 1 {
			return op.Inputs[0], s.advisory(op.Inputs[0])
		}
	}
	return "", false
}

func (s *CompiledSpec) advisory(input string) bool {
	return slices.ContainsFunc(s.Inputs, func(in Input) bool { return in.Name == input && in.Advisory() })
}

func InputStaleFact(input string) string { return InputStaleFacts + "." + input }

func checkAdvisoryInputs(spec *CompiledSpec) error {
	for _, input := range spec.AdvisoryInputs() {
		if !spec.watchesTheHeartbeatOf(input) {
			return &CompileError{Path: "inputs." + input + ".completenessRole",
				Message: fmt.Sprintf("advisory input %q needs a missing_heartbeat operator reading only it, whose duration says when it is stale", input)}
		}
	}
	return nil
}

func (s *CompiledSpec) watchesTheHeartbeatOf(input string) bool {
	return slices.ContainsFunc(s.Operators, func(op Operator) bool {
		return op.Kind == "missing_heartbeat" && slices.Equal(op.Inputs, []string{input})
	})
}
