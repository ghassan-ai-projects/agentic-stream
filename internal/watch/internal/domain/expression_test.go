package domain

import (
	"strings"
	"testing"
)

func TestValidateExpressionRefusesForbiddenSyntaxAndInvalidCEL(t *testing.T) {
	t.Parallel()
	for _, expression := range []string{"features.x > 1", "features.x > 1 && situation.y == 2"} {
		if err := ValidateExpression(expression); err != nil {
			t.Fatalf("%q: %v", expression, err)
		}
	}
	for expression, want := range map[string]string{"features.x;": "forbidden syntax", "features.{": "forbidden syntax", "features.x >": "not valid CEL"} {
		if err := ValidateExpression(expression); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: err = %v, want %q", expression, err, want)
		}
	}
}

func TestEvaluateMatchesFeaturesAndRejectsNonBooleanResults(t *testing.T) {
	t.Parallel()
	features := map[string]any{"temperature": 95}
	if matched, err := Evaluate("features.temperature > 90", features); err != nil || !matched {
		t.Fatalf("match = %v, %v", matched, err)
	}
	if matched, err := Evaluate("features.temperature > 100", features); err != nil || matched {
		t.Fatalf("no match = %v, %v", matched, err)
	}
	if _, err := Evaluate("features.temperature", features); err == nil || !strings.Contains(err.Error(), "must return bool") {
		t.Fatalf("non-boolean err = %v", err)
	}
	if _, err := Evaluate("features.absent > 1", features); err == nil {
		t.Fatal("a missing key was not an evaluation error")
	}
	if _, err := Evaluate("features.x >", features); err == nil {
		t.Fatal("invalid CEL evaluated")
	}
}
