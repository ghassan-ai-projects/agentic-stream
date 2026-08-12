// Package situations implements the Situation lifecycle reducer and versioned
// state machine.
package situations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/cel-go/cel"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Engine evaluates features and maintains Situation state for one partition.
type Engine struct {
	deploymentID string
	tenantID     string
	partitionID  int
	spec         *spec.CompiledSpec
	idGen        ids.Generator
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
	copy := *sit
	copy.Facts = cloneMap(sit.Facts)
	copy.Evidence = cloneSet(sit.Evidence)
	copy.ConditionStart = cloneTimes(sit.ConditionStart)
	blob, err := stateJSON(&copy)
	if err != nil {
		return Situation{}, nil, "", false, err
	}
	var document map[string]any
	if err := json.Unmarshal(blob, &document); err != nil {
		return Situation{}, nil, "", false, fmt.Errorf("decode current state: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, document)
	if err != nil {
		return Situation{}, nil, "", false, fmt.Errorf("digest current state: %w", err)
	}
	return copy, blob, digest, true, nil
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
func NewEngine(deploymentID, tenantID string, partitionID int, compiled *spec.CompiledSpec, idGen ids.Generator) (*Engine, error) {
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
	key := situationKey{partitionID: feature.PartitionID, entityType: feature.EntityType, entityID: feature.EntityID}
	sit, ok := e.active[key]
	if !ok {
		sit = e.newSituation(feature.PartitionID, feature.EntityType, feature.EntityID, feature.EventTime)
		e.active[key] = sit
	}
	completenessChanged := feature.Completeness != "" && feature.Completeness != sit.Completeness
	if feature.Completeness != "" {
		sit.Completeness = feature.Completeness
	}

	e.applyReducers(sit, feature)
	if len(feature.Metadata) > 0 {
		if sit.Facts == nil {
			sit.Facts = make(map[string]any)
		}
		sit.Facts["timer_provenance"] = cloneMap(feature.Metadata)
	}
	sit.LatestEventTime = feature.EventTime
	if feature.Traceparent != "" || !feature.TraceContinuation {
		sit.Traceparent = feature.Traceparent
		sit.Tracestate = feature.Tracestate
	}

	version, err := e.evaluate(ctx, sit, feature, watermark, completenessChanged)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, nil
	}

	return []Version{*version}, nil
}

func (e *Engine) newSituation(partitionID int, entityType, entityID string, eventTime time.Time) *Situation {
	identity := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s", e.tenantID, e.deploymentID, partitionID, e.spec.Situation.Type, entityType, entityID)
	hash := sha256.Sum256([]byte(identity))
	stableID := "sit_" + hex.EncodeToString(hash[:])
	return &Situation{
		SituationID:     stableID,
		TenantID:        e.tenantID,
		DeploymentID:    e.deploymentID,
		Type:            e.spec.Situation.Type,
		EntityType:      entityType,
		EntityID:        entityID,
		PartitionID:     partitionID,
		OccurrenceID:    "occ_" + hex.EncodeToString(hash[:]),
		Version:         0,
		Phase:           e.spec.Situation.InitialPhase,
		Severity:        e.initialSeverity(),
		Confidence:      1.0,
		Completeness:    string(operators.CompletenessProvisional),
		FirstEventTime:  eventTime,
		LatestEventTime: eventTime,
		Facts:           make(map[string]any),
		Evidence:        make(map[string]struct{}),
		ConditionStart:  make(map[string]time.Time),
		OpenedAt:        eventTime,
		UpdatedAt:       eventTime,
		Traceparent:     "",
		Tracestate:      "",
	}
}

func (e *Engine) initialSeverity() int {
	for _, p := range e.spec.Situation.Phases {
		if p.Name == e.spec.Situation.InitialPhase {
			return p.Severity
		}
	}
	return 0
}

func (e *Engine) applyReducers(sit *Situation, feature operators.Feature) {
	for _, r := range e.spec.Situation.Reducers {
		if r.Input != feature.OutputName {
			continue
		}
		switch r.Strategy {
		case "latest_event_time":
			_, exists := sit.Facts[r.Field]
			currentTime, _ := sit.Facts[r.Field+"_event_time"].(time.Time)
			if !exists || feature.EventTime.After(currentTime) {
				sit.Facts[r.Field] = feature.Value
				sit.Facts[r.Field+"_event_time"] = feature.EventTime
			}
		case "set_union":
			if sit.Evidence == nil {
				sit.Evidence = make(map[string]struct{})
			}
			for _, id := range feature.InputEventIDs {
				sit.Evidence[id] = struct{}{}
			}
		}
	}
}

func (e *Engine) evaluate(ctx context.Context, sit *Situation, feature operators.Feature, watermark time.Time, completenessChanged bool) (*Version, error) {
	features := e.buildFeaturesMap(sit)
	situation := e.buildSituationMap(sit)

	// Build a feature map for just this feature too for occurrence evaluation.
	_ = feature

	changed := false

	// Check occurrence close first if active.
	if sit.Version > 0 || sit.Phase != e.spec.Situation.InitialPhase {
		closed, err := e.evalBool(ctx, e.spec.Situation.Occurrence.CloseWhen, features, situation)
		if err != nil {
			return nil, err
		}
		if closed {
			if e.transition(sit, "resolved", watermark) {
				changed = true
			}
		}
	}

	// Evaluate transitions.
	for _, tr := range e.spec.Situation.Transitions {
		if tr.From != sit.Phase {
			continue
		}
		cond, err := e.evalBool(ctx, tr.When, features, situation)
		if err != nil {
			return nil, err
		}
		if cond {
			start := sit.ConditionStart[tr.From+"->"+tr.To]
			if start.IsZero() {
				start = feature.EventTime
				sit.ConditionStart[tr.From+"->"+tr.To] = start
			}
			minDur, _ := duration.Parse(tr.MinDuration)
			if feature.EventTime.Sub(start) >= minDur {
				if e.transition(sit, tr.To, watermark) {
					changed = true
				}
			}
		} else {
			delete(sit.ConditionStart, tr.From+"->"+tr.To)
		}
	}

	// Check occurrence open if not active.
	if sit.Version == 0 && sit.Phase == e.spec.Situation.InitialPhase {
		opened, err := e.evalBool(ctx, e.spec.Situation.Occurrence.OpenWhen, features, situation)
		if err != nil {
			return nil, err
		}
		if opened {
			sit.Version++
			changed = true
		}
	}
	if completenessChanged && sit.Version > 0 && !changed {
		sit.Version++
		sit.UpdatedAt = watermark
		changed = true
	}

	if !changed {
		return nil, nil
	}

	return e.materialize(sit, watermark)
}

func (e *Engine) transition(sit *Situation, to string, watermark time.Time) bool {
	if sit.Phase == to {
		return false
	}
	sit.PreviousPhase = sit.Phase
	sit.Phase = to
	sit.Version++
	sit.Severity = e.severityForPhase(to)
	sit.UpdatedAt = watermark
	sit.ConditionStart = make(map[string]time.Time)
	return true
}

func (e *Engine) severityForPhase(phase string) int {
	for _, p := range e.spec.Situation.Phases {
		if p.Name == phase {
			return p.Severity
		}
	}
	return 0
}

func (e *Engine) materialize(sit *Situation, watermark time.Time) (*Version, error) {
	facts := make(map[string]any, len(sit.Facts))
	for k, v := range sit.Facts {
		if !strings.HasSuffix(k, "_event_time") {
			facts[k] = v
		}
	}

	evidenceIDs := make([]string, 0, len(sit.Evidence))
	for id := range sit.Evidence {
		evidenceIDs = append(evidenceIDs, id)
	}
	sort.Strings(evidenceIDs)
	evidence := make([]any, len(evidenceIDs))
	for i, id := range evidenceIDs {
		evidence[i] = id
	}

	specDigest := e.spec.Digest
	if specDigest == "" {
		return nil, fmt.Errorf("compiled spec has no digest")
	}
	if _, err := canonicaljson.DecodeDigest(specDigest); err != nil {
		return nil, fmt.Errorf("invalid spec digest: %w", err)
	}
	snapshot := map[string]any{
		"situation_id":      sit.SituationID,
		"situation_version": sit.Version,
		"situation_type":    sit.Type,
		"tenant_id":         sit.TenantID,
		"entity":            map[string]any{"type": sit.EntityType, "id": sit.EntityID},
		"partition_id":      sit.PartitionID,
		"phase":             sit.Phase,
		"previous_phase":    sit.PreviousPhase,
		"severity":          sit.Severity,
		"confidence":        sit.Confidence,
		"completeness":      sit.Completeness,
		"facts":             facts,
		"evidence":          evidence,
		"event_horizon":     sit.LatestEventTime.Format(time.RFC3339Nano),
		"watermark":         watermark.Format(time.RFC3339Nano),
		"spec_digest":       specDigest,
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, snapshot); err != nil {
		return nil, fmt.Errorf("validate snapshot: %w", err)
	}
	snapshotJSON, err := canonicaljson.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		return nil, fmt.Errorf("digest snapshot: %w", err)
	}
	stateBlob, err := stateJSON(sit)
	if err != nil {
		return nil, fmt.Errorf("marshal persisted state: %w", err)
	}
	var stateDocument map[string]any
	if err := json.Unmarshal(stateBlob, &stateDocument); err != nil {
		return nil, fmt.Errorf("decode persisted state: %w", err)
	}
	stateDigest, err := canonicaljson.Digest(canonicaljson.DomainSituationState, stateDocument)
	if err != nil {
		return nil, fmt.Errorf("digest persisted state: %w", err)
	}

	return &Version{
		SituationID:     sit.SituationID,
		Type:            sit.Type,
		Version:         sit.Version,
		PreviousVersion: sit.Version - 1,
		Phase:           sit.Phase,
		PreviousPhase:   sit.PreviousPhase,
		Severity:        sit.Severity,
		Confidence:      sit.Confidence,
		Completeness:    sit.Completeness,
		EntityType:      sit.EntityType,
		EntityID:        sit.EntityID,
		EventHorizon:    sit.LatestEventTime,
		Watermark:       watermark,
		Facts:           facts,
		Evidence:        evidenceIDs,
		SnapshotJSON:    snapshotJSON,
		SnapshotSHA256:  digest,
		Traceparent:     sit.Traceparent,
		Tracestate:      sit.Tracestate,
		OccurrenceID:    sit.OccurrenceID,
		FirstEventTime:  sit.FirstEventTime,
		UpdatedAt:       sit.UpdatedAt,
		ConditionStart:  cloneTimes(sit.ConditionStart),
		StateJSON:       stateBlob,
		StateSHA256:     stateDigest,
	}, nil
}

func stateJSON(sit *Situation) ([]byte, error) {
	evidence := make([]string, 0, len(sit.Evidence))
	for id := range sit.Evidence {
		evidence = append(evidence, id)
	}
	sort.Strings(evidence)
	facts := make(map[string]any, len(sit.Facts))
	for key, value := range sit.Facts {
		if timestamp, ok := value.(time.Time); ok {
			facts[key] = timestamp.UTC().Format(time.RFC3339Nano)
			continue
		}
		facts[key] = value
	}
	conditionStart := make(map[string]string, len(sit.ConditionStart))
	for key, value := range sit.ConditionStart {
		conditionStart[key] = value.UTC().Format(time.RFC3339Nano)
	}
	blob, err := canonicaljson.Marshal(stateDocument(sit, facts, evidence, conditionStart))
	if err != nil {
		return nil, err
	}
	return blob, nil
}

func stateDocument(sit *Situation, facts map[string]any, evidence []string, conditionStart map[string]string) map[string]any {
	return map[string]any{
		"situation_id":    sit.SituationID,
		"occurrence_id":   sit.OccurrenceID,
		"partition_id":    sit.PartitionID,
		"version":         sit.Version,
		"facts":           facts,
		"evidence":        evidence,
		"condition_start": conditionStart,
		"traceparent":     sit.Traceparent,
		"tracestate":      sit.Tracestate,
	}
}

func cloneTimes(values map[string]time.Time) map[string]time.Time {
	clone := make(map[string]time.Time, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func (e *Engine) buildFeaturesMap(sit *Situation) map[string]any {
	features := make(map[string]any)
	for _, r := range e.spec.Situation.Reducers {
		switch r.Strategy {
		case "latest_event_time":
			if v, ok := sit.Facts[r.Field]; ok {
				features[r.Input] = v
			}
		case "set_union":
			evidence := make([]string, 0, len(sit.Evidence))
			for id := range sit.Evidence {
				evidence = append(evidence, id)
			}
			sort.Strings(evidence)
			features[r.Field] = evidence
		}
	}
	// Pre-populate defaults for every operator output so that CEL expressions
	// never fail on a missing key. Numeric features default to 0; heartbeat
	// detectors default to false.
	for _, op := range e.spec.Operators {
		if _, ok := features[op.Output]; ok {
			continue
		}
		switch op.Kind {
		case "missing_heartbeat":
			features[op.Output] = false
		default:
			features[op.Output] = 0.0
		}
	}
	return features
}

func (e *Engine) buildSituationMap(sit *Situation) map[string]any {
	return map[string]any{
		"phase":      sit.Phase,
		"severity":   sit.Severity,
		"confidence": sit.Confidence,
		"entity": map[string]any{
			"type": sit.EntityType,
			"id":   sit.EntityID,
		},
	}
}

func (e *Engine) evalBool(ctx context.Context, expr string, features, situation map[string]any) (bool, error) {
	_ = ctx
	if expr == "" {
		return false, nil
	}
	ast, issues := e.celEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("compile cel: %w", issues.Err())
	}
	prg, err := e.celEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("program cel: %w", err)
	}
	out, _, err := prg.Eval(map[string]any{
		"features":  features,
		"situation": situation,
	})
	if err != nil {
		return false, fmt.Errorf("eval cel: %w", err)
	}
	v, err := out.ConvertToNative(reflect.TypeOf(true))
	if err != nil {
		return false, fmt.Errorf("cel result not bool: %w", err)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("cel result not bool: %T", v)
	}
	return b, nil
}
