package domain

// FieldDerivation explains one Situation field from the spec: the reducer
// that writes it and the operator that computes the reducer's input.
type FieldDerivation struct {
	Field    string    `json:"field"`
	Strategy string    `json:"strategy"`
	Input    string    `json:"input"`
	Operator *Operator `json:"operator,omitempty"`
}

// FieldDerivations lists every reduced Situation field in spec order. An
// input that is not an operator output (a raw event field) has no operator.
func (c *CompiledSpec) FieldDerivations() []FieldDerivation {
	operators := make(map[string]Operator, len(c.Operators))
	for _, operator := range c.Operators {
		operators[operator.Output] = operator
	}
	derivations := make([]FieldDerivation, 0, len(c.Situation.Reducers))
	for _, reducer := range c.Situation.Reducers {
		derivation := FieldDerivation{Field: reducer.Field, Strategy: reducer.Strategy, Input: reducer.Input}
		if operator, ok := operators[reducer.Input]; ok {
			derivation.Operator = &operator
		}
		derivations = append(derivations, derivation)
	}
	return derivations
}
