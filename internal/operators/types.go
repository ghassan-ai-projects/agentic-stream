// Package operators implements deterministic, keyed stream operators.
package operators

import (
	"context"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Feature is an emitted operator result.
type Feature struct {
	FeatureID     string                 `json:"feature_id"`
	OperatorID    string                 `json:"operator_id"`
	OutputName    string                 `json:"output_name"`
	TenantID      string                 `json:"tenant_id"`
	EntityType    string                 `json:"entity_type"`
	EntityID      string                 `json:"entity_id"`
	PartitionID   int                    `json:"partition_id"`
	WindowStart   time.Time              `json:"window_start"`
	WindowEnd     time.Time              `json:"window_end"`
	Value         any                    `json:"value"`
	Unit          string                 `json:"unit,omitempty"`
	EventTime     time.Time              `json:"event_time"`
	Watermark     time.Time              `json:"watermark"`
	InputEventIDs []string               `json:"input_event_ids"`
	Completeness  string                 `json:"completeness"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// Runtime is the operator runtime interface used by the engine.
type Runtime interface {
	// ApplyEvent processes one normalized event and emits zero or more features.
	ApplyEvent(ctx context.Context, state *State, env contractsv1.Envelope, watermark time.Time) ([]Feature, *State, error)
	// ApplyTimer fires due timers and emits any resulting features.
	ApplyTimer(ctx context.Context, state *State, watermark time.Time) ([]Feature, *State, error)
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
}

// WindowState stores samples for windowed aggregates.
type WindowState struct {
	Samples   []Sample  `json:"samples"`
	LastEmit  time.Time `json:"last_emit"`
	WindowEnd time.Time `json:"window_end"`
}

// HeartbeatState stores the last seen heartbeat for a keyed entity.
type HeartbeatState struct {
	LastEventTime *time.Time `json:"last_event_time,omitempty"`
	LastEventID   string     `json:"last_event_id,omitempty"`
}

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
)
