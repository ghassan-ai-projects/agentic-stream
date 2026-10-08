package domain

import "testing"

func TestFieldDerivationsFollowReducerToOperator(t *testing.T) {
	t.Parallel()
	compiled := &CompiledSpec{
		Operators: []Operator{{Name: "temp_mean", Kind: "aggregate", Aggregate: "mean", Output: "temp_mean_5m"}},
		Situation: Situation{Reducers: []Reducer{
			{Field: "facts.temp_mean", Strategy: "latest_event_time", Input: "temp_mean_5m"},
			{Field: "facts.raw", Strategy: "latest_event_time", Input: "data.raw"},
		}},
	}
	derivations := compiled.FieldDerivations()
	if len(derivations) != 2 || derivations[0].Operator == nil || derivations[0].Operator.Name != "temp_mean" {
		t.Fatalf("derivations = %+v", derivations)
	}
	if derivations[1].Operator != nil || derivations[1].Input != "data.raw" {
		t.Fatalf("a raw input gained an operator: %+v", derivations[1])
	}
}
