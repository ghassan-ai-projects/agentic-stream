package domain

import "encoding/json"

// TriggerEvaluationRecord is one durable trigger evaluation: what the trigger
// saw (the delta), its score against the threshold, and why it decided.
type TriggerEvaluationRecord struct {
	TriggerID        string          `json:"trigger_id"`
	TriggerName      string          `json:"trigger_name"`
	SituationID      string          `json:"situation_id"`
	SituationVersion int             `json:"situation_version"`
	Score            float64         `json:"score"`
	Threshold        float64         `json:"threshold"`
	Lane             string          `json:"lane"`
	Outcome          string          `json:"outcome"`
	Reasons          []string        `json:"reasons"`
	Delta            json.RawMessage `json:"delta"`
	PolicySHA256     string          `json:"policy_sha256"`
	EvaluatedAt      string          `json:"evaluated_at"`
}
