package domain

import "time"

type Result struct {
	IntentID   string `json:"intent_id"`
	DecisionID string `json:"decision_id"`
	Result     string `json:"result"`
	Reason     string `json:"reason"`
	CommandID  string `json:"command_id,omitempty"`
	ApprovalID string `json:"approval_id,omitempty"`
}

type ApprovalAssertion struct {
	Approved         bool
	ApprovalID       string
	IntentID         string
	DecisionID       string
	TenantID         string
	SituationID      string
	SituationVersion int
	RiskClass        string
	IntentDigest     string
	DecisionDigest   string
	ExpiresAt        time.Time
	Nonce            string
	ApproverID       string
	RelayID          string
}

type IntentRecord struct {
	IntentID                string
	DecisionID              string
	EpisodeID               string
	EpisodeTenant           string
	EpisodeSituation        string
	EpisodeVersion          int
	SituationTenant         string
	DecisionSituation       string
	DecisionVersion         int
	TenantID                string
	SituationID             string
	SituationVersion        int
	IntentType              string
	RiskClass               string
	IntentJSON              []byte
	IntentSHA               []byte
	RateLimitPerHour        int
	RequiresApproval        int
	ExpiresAt               time.Time
	ExpiryUnreadable        bool
	PolicyStatus            string
	ValidationStatus        string
	DecisionJSON            []byte
	DecisionSHA             []byte
	Traceparent             string
	Tracestate              string
	EpisodeProducedDecision bool
	CurrentSituation        int
	LastMaterialVersion     int
	CurrentCompleteness     string
	ExecutorVersion         string
	SituationType           string
	PolicyEpoch             string
}
