package app

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

const (
	thermalMappingSpec    = "../../testdata/baseline/thermal-mapping.situation.yaml"
	thermalMappingChoices = "../../testdata/baseline/thermal-mapping-choices.json"
)

func TestBaselineDataReproducesTheChoicesTheThermalMappingOnceHardcoded(t *testing.T) {
	t.Parallel()
	choices := choicesOfExampleSpec(t, thermalMappingSpec).SingleIntent
	mapped := map[string]map[string]baselineChoice{
		"set_indicator":       choices["set_indicator"],
		"select_thermal_mode": choices["select_thermal_mode"],
	}
	encoded, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(thermalMappingChoices)
	if err != nil {
		t.Fatalf("read the pinned thermal choices: %v", err)
	}
	if !bytes.Equal(append(encoded, '\n'), want) {
		t.Fatalf("baseline data no longer reproduces the pinned thermal choices:\n%s", encoded)
	}
}
