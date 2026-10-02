package spec

import (
	_ "embed"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
	"strings"
)

func resolveReferences(spec *CompiledSpec) error {
	names, err := collectSpecNames(spec)
	if err != nil {
		return err
	}
	for _, op := range spec.Operators {
		if err := checkOperatorReferences(spec, op, names); err != nil {
			return err
		}
	}
	if err := checkSituationReferences(spec, names); err != nil {
		return err
	}
	for _, tr := range spec.Cognition.Triggers {
		if tr.Lane != "fast" && tr.Lane != "deep" {
			return &CompileError{Path: fmt.Sprintf("cognition.triggers.%s.lane", tr.Name), Message: fmt.Sprintf("invalid lane %q", tr.Lane)}
		}
	}
	return nil
}

// specNames are the declared names that references resolve against.
type specNames struct {
	inputs, windows, operatorOutputs, phases map[string]struct{}
}

// collectSpecNames requires unique input, window, operator, operator-output,
// and phase names, and every input to reference its registered event schema.
func collectSpecNames(spec *CompiledSpec) (specNames, error) {
	inputs, err := collectInputNames(spec.Inputs)
	if err != nil {
		return specNames{}, err
	}
	windows, err := uniqueNames(spec.Windows, func(w Window) string { return w.Name }, "windows", "window name")
	if err != nil {
		return specNames{}, err
	}
	outputs, err := collectOperatorOutputs(spec.Operators)
	if err != nil {
		return specNames{}, err
	}
	phases, err := uniqueNames(spec.Situation.Phases, func(p Phase) string { return p.Name }, "situation.phases", "phase name")
	if err != nil {
		return specNames{}, err
	}
	return specNames{inputs: inputs, windows: windows, operatorOutputs: outputs, phases: phases}, nil
}

// uniqueNames collects the names of items, rejecting a duplicate.
func uniqueNames[T any](items []T, name func(T) string, path, kind string) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if err := addUniqueName(seen, name(item), path, kind); err != nil {
			return nil, err
		}
	}
	return seen, nil
}

// collectInputNames requires unique input names, each referencing its
// registered event schema.
func collectInputNames(inputs []Input) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(inputs))
	for _, in := range inputs {
		if err := addUniqueName(seen, in.Name, "inputs", "input name"); err != nil {
			return nil, err
		}
		if err := checkInputSchema(in); err != nil {
			return nil, err
		}
	}
	return seen, nil
}

// collectOperatorOutputs requires unique operator names and outputs, and
// returns the outputs.
func collectOperatorOutputs(operators []Operator) (map[string]struct{}, error) {
	names := make(map[string]struct{}, len(operators))
	outputs := make(map[string]struct{}, len(operators))
	for _, op := range operators {
		if err := addUniqueName(names, op.Name, "operators", "operator name"); err != nil {
			return nil, err
		}
		if err := addUniqueName(outputs, op.Output, "operators", "operator output"); err != nil {
			return nil, err
		}
	}
	return outputs, nil
}

func addUniqueName(seen map[string]struct{}, name, path, kind string) error {
	if _, exists := seen[name]; exists {
		return &CompileError{Path: path, Message: fmt.Sprintf("duplicate %s %q", kind, name)}
	}
	seen[name] = struct{}{}
	return nil
}

func checkInputSchema(in Input) error {
	definition, ok := eventschema.Lookup(in.SchemaRef)
	if !ok {
		return &CompileError{Path: fmt.Sprintf("inputs.%s.schema", in.Name), Message: fmt.Sprintf("unknown event schema %q", in.SchemaRef)}
	}
	if definition.EventType != in.EventType || definition.SchemaVersion != in.SchemaVersion {
		return &CompileError{Path: fmt.Sprintf("inputs.%s.schema", in.Name), Message: "schema reference does not match eventType/schemaVersion"}
	}
	return nil
}

// checkOperatorReferences resolves an operator's inputs, window, and payload
// field. A data.* field must be declared by every input's schema, and an
// aggregate's unit must match the field's.
func checkOperatorReferences(spec *CompiledSpec, op Operator, names specNames) error {
	for _, in := range op.Inputs {
		_, isInput := names.inputs[in]
		_, isOutput := names.operatorOutputs[in]
		if !isInput && !isOutput {
			return &CompileError{Path: fmt.Sprintf("operators.%s.inputs", op.Name), Message: fmt.Sprintf("unknown input %q", in)}
		}
	}
	if op.Window != "" {
		if _, ok := names.windows[op.Window]; !ok {
			return &CompileError{Path: fmt.Sprintf("operators.%s.window", op.Name), Message: fmt.Sprintf("unknown window %q", op.Window)}
		}
	}
	fieldName, ok := strings.CutPrefix(op.Field, "data.")
	if !ok {
		return nil
	}
	for _, in := range operatorInputs(spec, op) {
		if err := checkOperatorField(op, fieldName, in); err != nil {
			return err
		}
	}
	return nil
}

// operatorInputs returns the spec inputs an operator reads, in operator
// input order.
func operatorInputs(spec *CompiledSpec, op Operator) []Input {
	var inputs []Input
	for _, inputName := range op.Inputs {
		for _, in := range spec.Inputs {
			if in.Name == inputName {
				inputs = append(inputs, in)
			}
		}
	}
	return inputs
}

// checkOperatorField requires the input's schema to declare the payload
// field, with a matching unit for an aggregate.
func checkOperatorField(op Operator, fieldName string, in Input) error {
	definition, _ := eventschema.Lookup(in.SchemaRef)
	field, exists := definition.Fields[fieldName]
	if !exists {
		return &CompileError{Path: fmt.Sprintf("operators.%s.field", op.Name), Message: fmt.Sprintf("payload field %q is not declared by schema %q", fieldName, in.SchemaRef)}
	}
	if op.Kind == "aggregate" && op.Unit != "" && op.Unit != field.Unit {
		return &CompileError{Path: fmt.Sprintf("operators.%s.unit", op.Name), Message: fmt.Sprintf("unit %q does not match field %q unit %q", op.Unit, fieldName, field.Unit)}
	}
	return nil
}

// checkSituationReferences resolves reducer inputs, the initial phase, and
// every transition's phases.
func checkSituationReferences(spec *CompiledSpec, names specNames) error {
	for _, r := range spec.Situation.Reducers {
		if _, ok := names.operatorOutputs[r.Input]; !ok {
			return &CompileError{Path: fmt.Sprintf("situation.reducers.%s.input", r.Field), Message: fmt.Sprintf("unknown operator output %q", r.Input)}
		}
	}
	if _, ok := names.phases[spec.Situation.InitialPhase]; !ok {
		return &CompileError{Path: "situation.initialPhase", Message: fmt.Sprintf("unknown phase %q", spec.Situation.InitialPhase)}
	}
	for _, t := range spec.Situation.Transitions {
		if _, ok := names.phases[t.From]; !ok {
			return &CompileError{Path: fmt.Sprintf("situation.transitions.%s-%s.from", t.From, t.To), Message: fmt.Sprintf("unknown phase %q", t.From)}
		}
		if _, ok := names.phases[t.To]; !ok {
			return &CompileError{Path: fmt.Sprintf("situation.transitions.%s-%s.to", t.From, t.To), Message: fmt.Sprintf("unknown phase %q", t.To)}
		}
	}
	return nil
}
