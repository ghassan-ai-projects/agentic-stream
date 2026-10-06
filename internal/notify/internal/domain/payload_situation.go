package domain

// SituationSuperseded records that a newer Situation version replaced work.
type SituationSuperseded struct {
	SituationID        string `json:"situation_id"`
	SupersededVersion  int    `json:"superseded_version"`
	ReplacementVersion int    `json:"replacement_version"`
	Reason             string `json:"reason"`
}

// EventType implements Payload.
func (SituationSuperseded) EventType() string { return TypeSituationSuperseded }

// ReconsiderationAdmitted records that a correction was admitted for a
// Situation whose earlier command and outcome it invalidates.
type ReconsiderationAdmitted struct {
	ReconsiderationID    string `json:"reconsideration_id"`
	SituationID          string `json:"situation_id"`
	SupersededVersion    int    `json:"superseded_version"`
	CorrectionVersion    int    `json:"correction_version"`
	InvalidatedCommandID string `json:"invalidated_command_id"`
	InvalidatedOutcomeID string `json:"invalidated_outcome_id"`
	TriggerID            string `json:"trigger_id"`
	SchedulerItemID      string `json:"scheduler_item_id"`
}

// EventType implements Payload.
func (ReconsiderationAdmitted) EventType() string { return TypeReconsiderationAdmitted }
