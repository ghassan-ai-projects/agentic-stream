// Package operators implements deterministic, keyed stream operators.
package operators

import "time"

// Feature is an emitted operator result.
type Feature struct {
	FeatureID  string `json:"feature_id"`
	OperatorID string `json:"operator_id"`
	OutputName string `json:"output_name"`
	TenantID   string `json:"tenant_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	// StateKey is the exact durable operator key. It is internal plumbing for
	// timer matching and is intentionally excluded from serialized evidence.
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

// State is the durable state blob for one operator instance (keyed by entity).
type State struct {
	OperatorID string
	StateKey   string
	Version    int
	Data       []byte
}

// Sample is one timestamped scalar input to a windowed operator.
type Sample struct {
	EventID   string    `json:"event_id"`
	EventTime time.Time `json:"event_time"`
	Value     float64   `json:"value"`
	BootID    string    `json:"boot_id,omitempty"`
}

// WindowState stores samples for windowed aggregates.
type WindowState struct {
	BootID    string    `json:"boot_id,omitempty"`
	Samples   []Sample  `json:"samples"`
	LastEmit  time.Time `json:"last_emit"`
	WindowEnd time.Time `json:"window_end"`
}

// HeartbeatState stores the last seen heartbeat for a keyed entity.
type HeartbeatState struct {
	BootID             string     `json:"boot_id,omitempty"`
	LastEventTime      *time.Time `json:"last_event_time,omitempty"`
	LastEventID        string     `json:"last_event_id,omitempty"`
	LastProcessingTime *time.Time `json:"last_processing_time,omitempty"`
	Traceparent        string     `json:"traceparent,omitempty"`
	Tracestate         string     `json:"tracestate,omitempty"`
}

// RuntimeState records boot admission for one logical entity. A boot is
// admitted once; an event from a previously seen boot is stale evidence and
// must not mutate active operator state.
type RuntimeState struct {
	CurrentBootID string   `json:"current_boot_id,omitempty"`
	SeenBootIDs   []string `json:"seen_boot_ids,omitempty"`
}

// RuntimeOperatorID identifies the metadata row persisted alongside normal
// operator state. It is not a user-declared operator.
const RuntimeOperatorID = "__agentic_stream_runtime__"

// PartitionState is the in-memory operator state for one partition.
type PartitionState struct {
	OperatorStates map[string]map[string]*OperatorStateBlob // operatorID -> stateKey -> blob
}

// Completeness describes how complete the feature is.
type Completeness string

const (
	CompletenessProvisional   Completeness = "provisional"
	CompletenessOnTime        Completeness = "on_time"
	CompletenessCorrected     Completeness = "corrected"
	CompletenessFinalByPolicy Completeness = "final_by_policy"
	CompletenessUncertain     Completeness = "uncertain"
)
