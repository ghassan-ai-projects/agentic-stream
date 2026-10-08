package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Evaluation is one trigger evaluation as scanned.
type Evaluation struct {
	TriggerID   string
	TriggerName string
	Score       float64
	Threshold   float64
	Lane        string
	DeltaJSON   []byte
}

var priorEvidenceKeys = []string{"prior_decision", "prior_command", "prior_outcome"}

func TakeReconsideration(item SchedulerItem, ev Evaluation, delta, snapshot map[string]any) (map[string]any, error) {
	prior, err := priorEvidence(delta)
	if err != nil {
		return nil, err
	}
	for _, key := range priorEvidenceKeys {
		delete(delta, key)
	}
	return reconsiderationDocument(item, prior, correctionDocument(ev, delta, snapshot), delta), nil
}

func reconsiderationDocument(item SchedulerItem, prior, correction, delta map[string]any) map[string]any {
	return map[string]any{
		"reconsideration_id":     contractsv1.DocumentString(delta, "reconsideration_id"),
		"situation_id":           item.SituationID,
		"superseded_version":     contractsv1.DocumentInt(delta, "superseded_version"),
		"correction_version":     contractsv1.DocumentInt(delta, "correction_version"),
		"invalidated_command_id": contractsv1.DocumentString(delta, "invalidated_command_id"),
		"invalidated_outcome_id": contractsv1.DocumentString(delta, "invalidated_outcome_id"),
		"prior_decision":         prior["prior_decision"],
		"commands":               []any{prior["prior_command"]},
		"outcomes":               []any{prior["prior_outcome"]},
		"correction":             correction,
	}
}

func priorEvidence(delta map[string]any) (map[string]any, error) {
	prior := make(map[string]any, len(priorEvidenceKeys))
	for _, key := range priorEvidenceKeys {
		document, ok := delta[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("reconsideration delta %s is required", key)
		}
		prior[key] = document
	}
	return prior, nil
}

func correctionDocument(ev Evaluation, delta, snapshot map[string]any) map[string]any {
	correction := copyDocument(snapshot)
	if nested, ok := delta["correction"].(map[string]any); ok {
		correction = copyDocument(nested)
	}
	correction["reason"] = ev.TriggerName
	correction["invalidates"] = []string{contractsv1.DocumentString(delta, "invalidated_command_id")}
	correction["superseded_version"] = contractsv1.DocumentInt(delta, "superseded_version")
	correction["correction_version"] = contractsv1.DocumentInt(delta, "correction_version")
	return correction
}

func copyDocument(document map[string]any) map[string]any {
	copy := make(map[string]any, len(document))
	for key, value := range document {
		copy[key] = value
	}
	return copy
}
