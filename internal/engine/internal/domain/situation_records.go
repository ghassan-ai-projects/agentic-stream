package domain

import "encoding/json"

type SituationSummary struct {
	SituationID         string `json:"situation_id"`
	DeploymentID        string `json:"deployment_id"`
	SituationType       string `json:"situation_type"`
	EntityType          string `json:"entity_type"`
	EntityID            string `json:"entity_id"`
	OccurrenceID        string `json:"occurrence_id"`
	CurrentVersion      int    `json:"current_version"`
	LastMaterialVersion int    `json:"last_material_version"`
	Phase               string `json:"phase"`
	Status              string `json:"status"`
	FirstEventTime      string `json:"first_event_time"`
	LatestEventTime     string `json:"latest_event_time"`
}

type SituationVersionRecord struct {
	SituationID     string          `json:"situation_id"`
	DeploymentID    string          `json:"deployment_id"`
	Version         int             `json:"version"`
	PreviousVersion *int            `json:"previous_version,omitempty"`
	Phase           string          `json:"phase"`
	PreviousPhase   string          `json:"previous_phase,omitempty"`
	Severity        int             `json:"severity"`
	Confidence      float64         `json:"confidence"`
	Completeness    string          `json:"completeness"`
	EventHorizon    string          `json:"event_horizon"`
	Watermark       string          `json:"watermark,omitempty"`
	ValidFrom       string          `json:"valid_from"`
	ValidUntil      string          `json:"valid_until,omitempty"`
	SnapshotSHA256  string          `json:"snapshot_sha256"`
	Snapshot        json.RawMessage `json:"snapshot"`
	LineageID       string          `json:"lineage_id"`
	Evidence        []string        `json:"evidence"`
	CreatedAt       string          `json:"created_at"`
}
