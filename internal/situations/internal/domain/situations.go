package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Engine evaluates features and maintains Situation state for one partition.
type Engine struct {
	deploymentID string
	tenantID     string
	partitionID  int
	spec         *spec.CompiledSpec
	idGen        sources.Generator
	celEnv       *cel.Env

	active map[situationKey]*Situation
}

type situationKey struct {
	partitionID int
	entityType  string
	entityID    string
}

// Situation is the mutable current state for one occurrence.
type Situation struct {
	SituationID     string
	TenantID        string
	DeploymentID    string
	Type            string
	EntityType      string
	EntityID        string
	PartitionID     int
	OccurrenceID    string
	Version         int
	Phase           string
	PreviousPhase   string
	Severity        int
	Confidence      float64
	Completeness    string
	FirstEventTime  time.Time
	LatestEventTime time.Time
	Facts           map[string]any
	Evidence        map[string]struct{}
	ConditionStart  map[string]time.Time // transition key -> first true event time
	OpenedAt        time.Time
	UpdatedAt       time.Time
	Traceparent     string
	Tracestate      string
}

// Version is an immutable Situation version.
type Version struct {
	SituationID     string
	Type            string
	Version         int
	PreviousVersion int
	Phase           string
	PreviousPhase   string
	Severity        int
	Confidence      float64
	Completeness    string
	EntityType      string
	EntityID        string
	EventHorizon    time.Time
	Watermark       time.Time
	Traceparent     string
	Tracestate      string
	OccurrenceID    string
	FirstEventTime  time.Time
	UpdatedAt       time.Time
	ConditionStart  map[string]time.Time
	StateJSON       []byte
	StateSHA256     string
	Facts           map[string]any
	Evidence        []string
	SnapshotJSON    []byte
	SnapshotSHA256  string
}

// Restore loads one durable current Situation into the in-memory index.
func (e *Engine) Restore(s Situation) error {
	if s.SituationID == "" || s.EntityType == "" || s.EntityID == "" {
		return fmt.Errorf("restore situation requires identity")
	}
	if s.Facts == nil {
		s.Facts = make(map[string]any)
	}
	if s.Evidence == nil {
		s.Evidence = make(map[string]struct{})
	}
	if s.ConditionStart == nil {
		s.ConditionStart = make(map[string]time.Time)
	}
	e.active[situationKey{partitionID: s.PartitionID, entityType: s.EntityType, entityID: s.EntityID}] = &s
	return nil
}

// Reset discards the in-memory index so it can be rebuilt from durable state
// after a transaction rollback.
func (e *Engine) Reset() {
	e.active = make(map[situationKey]*Situation)
}

// CurrentState returns a copy of the current Situation and its canonical
// reducer-state payload for transactional persistence.
func (e *Engine) CurrentState(partitionID int, entityType, entityID string) (Situation, []byte, string, bool, error) {
	sit, ok := e.active[situationKey{partitionID: partitionID, entityType: entityType, entityID: entityID}]
	if !ok {
		return Situation{}, nil, "", false, nil
	}
	copy := cloneSituation(sit)
	blob, err := stateJSON(&copy)
	if err != nil {
		return Situation{}, nil, "", false, err
	}
	digest, err := stateDigest(blob)
	if err != nil {
		return Situation{}, nil, "", false, err
	}
	return copy, blob, digest, true, nil
}

// cloneSituation copies the Situation with its own facts, evidence and
// condition timers.
func cloneSituation(sit *Situation) Situation {
	copy := *sit
	copy.Facts = cloneMap(sit.Facts)
	copy.Evidence = cloneSet(sit.Evidence)
	copy.ConditionStart = cloneTimes(sit.ConditionStart)
	return copy
}

func stateDigest(blob []byte) (string, error) {
	var document map[string]any
	if err := json.Unmarshal(blob, &document); err != nil {
		return "", fmt.Errorf("decode current state: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		return "", fmt.Errorf("digest current state: %w", err)
	}
	return digest, nil
}

func cloneMap(values map[string]any) map[string]any {
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneSet(values map[string]struct{}) map[string]struct{} {
	clone := make(map[string]struct{}, len(values))
	for key := range values {
		clone[key] = struct{}{}
	}
	return clone
}

// NewEngine creates a situation engine.
func NewEngine(deploymentID, tenantID string, partitionID int, compiled *spec.CompiledSpec, idGen sources.Generator) (*Engine, error) {
	env, err := spec.NewCELEnv()
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}
	return &Engine{
		deploymentID: deploymentID,
		tenantID:     tenantID,
		partitionID:  partitionID,
		spec:         compiled,
		idGen:        idGen,
		celEnv:       env,
		active:       make(map[situationKey]*Situation),
	}, nil
}

// ApplyFeature updates situation state with one emitted feature.
func (e *Engine) ApplyFeature(ctx context.Context, feature operators.Feature, watermark time.Time) ([]Version, error) {
	sit := e.situationForFeature(feature)
	completenessChanged := e.applyFeatureEvidence(sit, feature)

	version, err := e.evaluate(ctx, sit, feature, watermark, completenessChanged)
	if err != nil {
		return nil, fmt.Errorf("evaluate situation: %w", err)
	}
	if version == nil {
		return nil, nil
	}

	return []Version{*version}, nil
}

func (e *Engine) situationForFeature(feature operators.Feature) *Situation {
	key := situationKey{partitionID: feature.PartitionID, entityType: feature.EntityType, entityID: feature.EntityID}
	sit, ok := e.active[key]
	if !ok {
		sit = e.newSituation(feature.PartitionID, feature.EntityType, feature.EntityID, feature.EventTime)
		e.active[key] = sit
	}
	return sit
}

func (e *Engine) newSituation(partitionID int, entityType, entityID string, eventTime time.Time) *Situation {
	identity := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s", e.tenantID, e.deploymentID, partitionID, e.spec.Situation.Type, entityType, entityID)
	hash := hex.EncodeToString(sha256Sum(identity))
	return &Situation{
		SituationID: sources.PrefixSituation + hash, TenantID: e.tenantID, DeploymentID: e.deploymentID,
		Type: e.spec.Situation.Type, EntityType: entityType, EntityID: entityID, PartitionID: partitionID,
		OccurrenceID: "occ_" + hash, Version: 0, Phase: e.spec.Situation.InitialPhase,
		Severity: e.initialSeverity(), Confidence: 1.0, Completeness: string(operators.CompletenessProvisional),
		FirstEventTime: eventTime, LatestEventTime: eventTime,
		Facts: make(map[string]any), Evidence: make(map[string]struct{}), ConditionStart: make(map[string]time.Time),
		OpenedAt: eventTime, UpdatedAt: eventTime, Traceparent: "", Tracestate: "",
	}
}

func sha256Sum(value string) []byte {
	hash := sha256.Sum256([]byte(value))
	return hash[:]
}

func (e *Engine) initialSeverity() int {
	for _, p := range e.spec.Situation.Phases {
		if p.Name == e.spec.Situation.InitialPhase {
			return p.Severity
		}
	}
	return 0
}
