package campaign

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed canned.json
var cannedJSON []byte

var loadCanned = sync.OnceValues(func() (canned, error) {
	var c canned
	if err := json.Unmarshal(cannedJSON, &c); err != nil {
		return canned{}, fmt.Errorf("decode the canned answers: %w", err)
	}
	return c, nil
})

type canned struct {
	Diagnoses []diagnosis       `json:"diagnoses"`
	Drafts    map[string]string `json:"drafts"`
}

type diagnosis struct {
	Rises         string  `json:"rises"`
	Above         float64 `json:"above"`
	DiagnosisCode string  `json:"diagnosis_code"`
	Intent        *intent `json:"intent"`
}

type intent struct {
	Type       string         `json:"type"`
	Risk       string         `json:"risk"`
	Parameters map[string]any `json:"parameters"`
}

type answer struct {
	DiagnosisCode string  `json:"diagnosis_code"`
	Intent        *intent `json:"intent,omitempty"`
}

var unknown = answer{DiagnosisCode: "unknown"}

func diagnose(facts map[string]float64) (answer, error) {
	c, err := loadCanned()
	if err != nil {
		return answer{}, err
	}
	for _, d := range c.Diagnoses {
		if rise(facts, d.Rises) > d.Above {
			return answer{DiagnosisCode: d.DiagnosisCode, Intent: d.Intent}, nil
		}
	}
	return unknown, nil
}

func rise(facts map[string]float64, feature string) float64 {
	baseline := facts[feature+"_mean_24h"]
	if baseline <= 0 {
		return 0
	}
	return facts[feature+"_mean_15m"] / baseline
}

func draftCondition(diagnosisCode string) (string, error) {
	c, err := loadCanned()
	if err != nil {
		return "", err
	}
	return c.Drafts[diagnosisCode], nil
}
