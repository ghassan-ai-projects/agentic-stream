package domain

// CommandDispatched records that a dispatch result was recorded for a command.
type CommandDispatched struct {
	CommandID string `json:"command_id"`
	IntentID  string `json:"intent_id"`
	OutcomeID string `json:"outcome_id"`
	Status    string `json:"status"`
}

// EventType implements Payload.
func (CommandDispatched) EventType() string { return TypeCommandDispatched }

// OutcomeRecorded records that an outcome was recorded for a command.
type OutcomeRecorded struct {
	IntentID             string `json:"intent_id"`
	CommandID            string `json:"command_id"`
	OutcomeID            string `json:"outcome_id"`
	OutcomeDigest        string `json:"outcome_digest"`
	Status               string `json:"status"`
	ReconciliationStatus string `json:"reconciliation_status"`
}

// EventType implements Payload.
func (OutcomeRecorded) EventType() string { return TypeOutcomeRecorded }

// OutcomeReconciled records that independent evidence settled an outcome.
type OutcomeReconciled struct {
	IntentID              string `json:"intent_id"`
	CommandID             string `json:"command_id"`
	OutcomeID             string `json:"outcome_id"`
	OutcomeDigest         string `json:"outcome_digest"`
	FinalStatus           string `json:"final_status"`
	ReconciliationStatus  string `json:"reconciliation_status"`
	Verdict               string `json:"verdict"`
	ReconciliationVersion int    `json:"reconciliation_version"`
}

// EventType implements Payload.
func (OutcomeReconciled) EventType() string { return TypeOutcomeReconciled }
