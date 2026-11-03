package domain

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	pumpSpecPath     = "../../../../examples/pump-station/pump-station.situation.yaml"
	pumpPlaybookPath = "../../../../examples/pump-station/testdata/site-a.playbook.yaml"
)

func pumpRules(t *testing.T) *Rules {
	t.Helper()
	compiled, err := spec.CompileFile(t.Context(), pumpSpecPath)
	if err != nil {
		t.Fatal(err)
	}
	playbook, err := spec.CompilePlaybookFile(compiled, pumpPlaybookPath)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := NewRules(compiled, playbook)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func pumpFeatures(rules *Rules, vibration, axial, current float64) map[string]any {
	return rules.Features(map[string]any{
		"facts.vibration_mean_15m": vibration, "facts.vibration_mean_24h": 3.0,
		"facts.axial_mean_15m": axial, "facts.axial_mean_24h": 2.0,
		"facts.current_mean_15m": current, "facts.current_mean_24h": 30.0,
	}, nil)
}

var bearingWear = RuleKey{ID: "bearing-wear", Version: 1}
