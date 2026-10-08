package domain

import "encoding/json"

// CommandView is one command an intent produced, with every outcome the
// provider reported and every verification of its effect.
type CommandView struct {
	CommandID     string             `json:"command_id"`
	EffectorRoute string             `json:"effector_route"`
	Target        string             `json:"target"`
	Status        string             `json:"status"`
	CreatedAt     string             `json:"created_at"`
	UpdatedAt     string             `json:"updated_at"`
	Outcomes      []OutcomeView      `json:"outcomes"`
	Verifications []VerificationView `json:"verifications"`
}

// OutcomeView is one outcome of a command as the provider reported it.
type OutcomeView struct {
	OutcomeID            string          `json:"outcome_id"`
	Ordinal              int             `json:"ordinal"`
	Status               string          `json:"status"`
	ReconciliationStatus string          `json:"reconciliation_status,omitempty"`
	ProviderResult       json.RawMessage `json:"provider_result,omitempty"`
	OccurredAt           string          `json:"occurred_at"`
}

// VerificationView is one verification of a command's effect and its verdict.
type VerificationView struct {
	VerificationID string          `json:"verification_id"`
	OutcomeID      string          `json:"outcome_id,omitempty"`
	Status         string          `json:"status"`
	Verdict        json.RawMessage `json:"verdict,omitempty"`
	UpdatedAt      string          `json:"updated_at"`
}
