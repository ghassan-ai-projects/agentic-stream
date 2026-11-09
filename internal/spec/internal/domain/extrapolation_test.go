package domain_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

const aggregateOperator = "    kind: aggregate\n    inputs: [temp]\n    field: data.value\n    window: w1\n    aggregate: mean\n"

func extrapolateOperator(configuration string) string {
	return "    kind: extrapolate\n    inputs: [temp]\n    field: data.value\n    window: w1\n    configuration:\n" + configuration
}

func TestAnExtrapolateOperatorCompilesAndKeepsItsConfigurationThroughItsSealedForm(t *testing.T) {
	t.Parallel()
	compiled, err := compileSource(t, editedSpec(aggregateOperator, extrapolateOperator("      limit: 150\n      fit: quadratic\n      minPoints: 30\n      maxHorizon: 6h\n")))
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Extrapolation{Limit: 150, Fit: domain.FitQuadratic, MinPoints: 30, MaxHorizon: 6 * time.Hour}
	if got, err := compiled.Operators[0].Extrapolation(); err != nil || got != want {
		t.Fatalf("compiled extrapolation = %+v, %v; want %+v", got, err, want)
	}
	var restored domain.CompiledSpec
	if err := json.Unmarshal(compiled.CanonicalJSON, &restored); err != nil {
		t.Fatal(err)
	}
	if got, err := restored.Operators[0].Extrapolation(); err != nil || got != want {
		t.Fatalf("restored extrapolation = %+v, %v; want %+v", got, err, want)
	}
}

func TestAnExtrapolateOperatorDefaultsToALinearFitOfTwoPoints(t *testing.T) {
	t.Parallel()
	compiled, err := compileSource(t, editedSpec(aggregateOperator, extrapolateOperator("      limit: 150\n")))
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Extrapolation{Limit: 150, Fit: domain.FitLinear, MinPoints: 2}
	if got, err := compiled.Operators[0].Extrapolation(); err != nil || got != want {
		t.Fatalf("extrapolation = %+v, %v; want %+v", got, err, want)
	}
}

func TestAnExtrapolateOperatorWithABadConfigurationDoesNotCompile(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"no limit":                 extrapolateOperator("      fit: linear\n"),
		"an unknown fit":           extrapolateOperator("      limit: 150\n      fit: cubic\n"),
		"too few points for a fit": extrapolateOperator("      limit: 150\n      fit: quadratic\n      minPoints: 2\n"),
		"a fractional minPoints":   extrapolateOperator("      limit: 150\n      minPoints: 2.5\n"),
		"a negative maxHorizon":    extrapolateOperator("      limit: 150\n      maxHorizon: -1h\n"),
		"no window":                "    kind: extrapolate\n    inputs: [temp]\n    field: data.value\n    configuration:\n      limit: 150\n",
	}
	for name, operator := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := compileSource(t, editedSpec(aggregateOperator, operator))
			var compileErr *domain.CompileError
			if !errors.As(err, &compileErr) {
				t.Fatalf("error = %v, want a compile error", err)
			}
		})
	}
}
