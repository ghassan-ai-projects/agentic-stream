package domain

import "encoding/json"

// IntentView is one intent as an operator inspects it: the proposal, its
// policy status and every policy evaluation with its reason.
type IntentView struct {
	IntentID         string                 `json:"intent_id"`
	DecisionID       string                 `json:"decision_id"`
	SituationID      string                 `json:"situation_id"`
	SituationVersion int                    `json:"situation_version"`
	IntentType       string                 `json:"intent_type"`
	RiskClass        string                 `json:"risk_class"`
	PolicyStatus     string                 `json:"policy_status"`
	RequiresApproval bool                   `json:"requires_approval"`
	ExpiresAt        string                 `json:"expires_at"`
	Intent           json.RawMessage        `json:"intent"`
	CreatedAt        string                 `json:"created_at"`
	Evaluations      []PolicyEvaluationView `json:"evaluations"`
}

// PolicyEvaluationView is one policy evaluation of an intent and its reason.
type PolicyEvaluationView struct {
	EvaluationID     string `json:"evaluation_id"`
	Result           string `json:"result"`
	Reason           string `json:"reason"`
	PolicyVersion    string `json:"policy_version"`
	PolicyDigest     string `json:"policy_digest"`
	CommandID        string `json:"command_id,omitempty"`
	ApprovalID       string `json:"approval_id,omitempty"`
	SituationVersion int    `json:"situation_version"`
	EvaluatedAt      string `json:"evaluated_at"`
}
