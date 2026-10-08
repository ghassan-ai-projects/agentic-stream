package domain

import "encoding/json"

type DecisionView struct {
	DecisionID       string          `json:"decision_id"`
	AttemptID        string          `json:"attempt_id,omitempty"`
	Fence            int64           `json:"fence"`
	Ordinal          int             `json:"ordinal"`
	SituationVersion int             `json:"situation_version"`
	ValidationStatus string          `json:"validation_status"`
	RejectionReason  string          `json:"rejection_reason,omitempty"`
	DecisionSHA256   string          `json:"decision_sha256"`
	Decision         json.RawMessage `json:"decision"`
	CreatedAt        string          `json:"created_at"`
}
