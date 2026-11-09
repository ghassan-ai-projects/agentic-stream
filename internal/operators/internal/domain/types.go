package domain

import "time"

type Feature struct {
	FeatureID         string                 `json:"feature_id"`
	OperatorID        string                 `json:"operator_id"`
	OutputName        string                 `json:"output_name"`
	TenantID          string                 `json:"tenant_id"`
	EntityType        string                 `json:"entity_type"`
	EntityID          string                 `json:"entity_id"`
	StateKey          string                 `json:"-"`
	BootID            string                 `json:"boot_id,omitempty"`
	PartitionID       int                    `json:"partition_id"`
	WindowStart       time.Time              `json:"window_start"`
	WindowEnd         time.Time              `json:"window_end"`
	Value             any                    `json:"value"`
	Unit              string                 `json:"unit,omitempty"`
	EventTime         time.Time              `json:"event_time"`
	Watermark         time.Time              `json:"watermark"`
	InputEventIDs     []string               `json:"input_event_ids"`
	Completeness      string                 `json:"completeness"`
	Traceparent       string                 `json:"traceparent,omitempty"`
	Tracestate        string                 `json:"tracestate,omitempty"`
	TraceContinuation bool                   `json:"trace_continuation,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

type State struct {
	OperatorID string
	StateKey   string
	Version    int
	Data       []byte
}

type Sample struct {
	EventID   string    `json:"event_id"`
	EventTime time.Time `json:"event_time"`
	Value     float64   `json:"value"`
	BootID    string    `json:"boot_id,omitempty"`
}

type WindowState struct {
	BootID    string    `json:"boot_id,omitempty"`
	Samples   []Sample  `json:"-"`
	LastEmit  time.Time `json:"last_emit"`
	WindowEnd time.Time `json:"window_end"`
	changes   sampleChanges
}

type HeartbeatState struct {
	BootID             string     `json:"boot_id,omitempty"`
	LastEventTime      *time.Time `json:"last_event_time,omitempty"`
	LastEventID        string     `json:"last_event_id,omitempty"`
	LastProcessingTime *time.Time `json:"last_processing_time,omitempty"`
	Traceparent        string     `json:"traceparent,omitempty"`
	Tracestate         string     `json:"tracestate,omitempty"`
}

type RuntimeState struct {
	Fences map[string]*BootFence `json:"fences,omitempty"`
}

type BootFence struct {
	CurrentBootID string   `json:"current_boot_id,omitempty"`
	SeenBootIDs   []string `json:"seen_boot_ids,omitempty"`
}

const RuntimeOperatorID = "__agentic_stream_runtime__"

type PartitionState struct {
	OperatorStates map[string]map[string]*OperatorStateBlob
}

type Completeness string

const (
	CompletenessProvisional   Completeness = "provisional"
	CompletenessOnTime        Completeness = "on_time"
	CompletenessCorrected     Completeness = "corrected"
	CompletenessFinalByPolicy Completeness = "final_by_policy"
	CompletenessUncertain     Completeness = "uncertain"
)
