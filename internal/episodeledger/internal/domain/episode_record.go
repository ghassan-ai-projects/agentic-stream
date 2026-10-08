package domain

import "encoding/json"

// EpisodeRecord is one episode as an operator inspects it: what it reasoned
// over, its lifecycle, every attempt and every result the ledger refused.
type EpisodeRecord struct {
	EpisodeID        string            `json:"episode_id"`
	SchedulerItemID  string            `json:"scheduler_item_id"`
	SituationID      string            `json:"situation_id"`
	SituationVersion int               `json:"situation_version"`
	ExecutorName     string            `json:"executor_name"`
	ExecutorVersion  string            `json:"executor_version"`
	ModelPolicy      string            `json:"model_policy"`
	PromptVersion    string            `json:"prompt_version"`
	DispatchPolicy   string            `json:"dispatch_policy"`
	SnapshotSHA256   string            `json:"snapshot_sha256"`
	LifecycleStatus  string            `json:"lifecycle_status"`
	CurrentAttemptID string            `json:"current_attempt_id,omitempty"`
	CurrentFence     int64             `json:"current_fence"`
	AcceptedAt       string            `json:"accepted_at"`
	EndedAt          string            `json:"ended_at,omitempty"`
	Terminal         json.RawMessage   `json:"terminal,omitempty"`
	Attempts         []AttemptView     `json:"attempts"`
	Rejections       []RejectionRecord `json:"rejections"`
}

// AttemptView is one fenced worker attempt of an episode, as inspected.
type AttemptView struct {
	AttemptID string          `json:"attempt_id"`
	Fence     int64           `json:"fence"`
	Status    string          `json:"status"`
	StartedAt string          `json:"started_at"`
	EndedAt   string          `json:"ended_at,omitempty"`
	Terminal  json.RawMessage `json:"terminal,omitempty"`
}
