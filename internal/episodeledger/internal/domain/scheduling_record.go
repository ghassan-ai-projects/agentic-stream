package domain

import "encoding/json"

// SchedulingRecord is what became of one admitted trigger evaluation: its
// scheduler item, the episode it admitted (if any) and any results the
// ledger refused for that episode.
type SchedulingRecord struct {
	SchedulerItemID         string            `json:"scheduler_item_id"`
	Status                  string            `json:"status"`
	Lane                    string            `json:"lane"`
	Priority                float64           `json:"priority"`
	ExpiresAt               string            `json:"expires_at"`
	UpdatedAt               string            `json:"updated_at"`
	EpisodeID               string            `json:"episode_id,omitempty"`
	EpisodeStatus           string            `json:"episode_status,omitempty"`
	EpisodeSituationVersion int               `json:"episode_situation_version,omitempty"`
	Rejections              []RejectionRecord `json:"rejections"`
}

// RejectionRecord is one worker result the ledger refused, with its reason.
type RejectionRecord struct {
	Reason    string          `json:"reason"`
	AttemptID string          `json:"attempt_id,omitempty"`
	Fence     int64           `json:"fence"`
	Details   json.RawMessage `json:"details"`
	CreatedAt string          `json:"created_at"`
}
