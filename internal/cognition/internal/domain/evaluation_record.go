package domain

import "encoding/json"

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
