package operators

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Runtime applies compiled operators to normalized events.
type OperatorRuntime struct {
	deploymentID string
	spec         *spec.CompiledSpec
	idGen        ids.Generator

	// operators keyed by direct input name.
	byInput map[string][]*operatorInstance
}

type operatorInstance struct {
	def    spec.Operator
	window *windowConfig
}

type windowConfig struct {
	size  time.Duration
	slide time.Duration
	emit  string
}

// NewRuntime creates an operator runtime for the compiled spec.
func NewOperatorRuntime(deploymentID string, compiled *spec.CompiledSpec, idGen ids.Generator) (*OperatorRuntime, error) {
	windows := make(map[string]spec.Window, len(compiled.Windows))
	for _, w := range compiled.Windows {
		windows[w.Name] = w
	}

	byInput := make(map[string][]*operatorInstance)
	for i := range compiled.Operators {
		op := compiled.Operators[i]
		inst := &operatorInstance{def: op}
		if op.Window != "" {
			w, ok := windows[op.Window]
			if !ok {
				return nil, fmt.Errorf("operator %s references unknown window %s", op.Name, op.Window)
			}
			cfg, err := newWindowConfig(w)
			if err != nil {
				return nil, fmt.Errorf("operator %s window: %w", op.Name, err)
			}
			inst.window = cfg
		}
		for _, in := range op.Inputs {
			byInput[in] = append(byInput[in], inst)
		}
	}

	return &OperatorRuntime{
		deploymentID: deploymentID,
		spec:         compiled,
		idGen:        idGen,
		byInput:      byInput,
	}, nil
}

func newWindowConfig(w spec.Window) (*windowConfig, error) {
	cfg := &windowConfig{emit: w.Emit}
	if cfg.emit == "" {
		cfg.emit = "on_close"
	}

	switch w.Kind {
	case "tumbling":
		d, err := parseDuration(w.Size)
		if err != nil {
			return nil, fmt.Errorf("tumbling size: %w", err)
		}
		cfg.size = d
	case "sliding":
		size, err := parseDuration(w.Size)
		if err != nil {
			return nil, fmt.Errorf("sliding size: %w", err)
		}
		slide, err := parseDuration(w.Slide)
		if err != nil {
			return nil, fmt.Errorf("sliding slide: %w", err)
		}
		cfg.size = size
		cfg.slide = slide
	default:
		return nil, fmt.Errorf("unsupported window kind %q", w.Kind)
	}
	return cfg, nil
}

func parseDuration(s string) (time.Duration, error) {
	d, err := duration.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return d, nil
}

// OperatorStateBlob is the JSON-serializable state for one operator key.
type OperatorStateBlob struct {
	Window    *WindowState    `json:"window,omitempty"`
	Heartbeat *HeartbeatState `json:"heartbeat,omitempty"`
}

// ApplyEvent processes one event against all operators that consume its input.
func (r *OperatorRuntime) ApplyEvent(ctx context.Context, ps *PartitionState, env contractsv1.Envelope, watermark time.Time) ([]Feature, *PartitionState, error) {
	if ps == nil {
		ps = &PartitionState{OperatorStates: make(map[string]map[string]*OperatorStateBlob)}
	}

	inputName, ok := r.inputForEvent(env)
	if !ok {
		return nil, ps, nil
	}

	var features []Feature
	for _, inst := range r.byInput[inputName] {
		if len(inst.def.Inputs) > 0 && inst.def.Inputs[0] != inputName {
			continue // Only direct inputs supported for now.
		}
		fs, err := r.applyOperator(ctx, ps, inst, env, watermark)
		if err != nil {
			return nil, ps, err
		}
		features = append(features, fs...)
	}

	return features, ps, nil
}

func (r *OperatorRuntime) inputForEvent(env contractsv1.Envelope) (string, bool) {
	for _, in := range r.spec.Inputs {
		if in.EventType == env.Type {
			return in.Name, true
		}
	}
	return "", false
}

func (r *OperatorRuntime) applyOperator(ctx context.Context, ps *PartitionState, inst *operatorInstance, env contractsv1.Envelope, watermark time.Time) ([]Feature, error) {
	stateKey := env.Entity.ID
	blob := r.getBlob(ps, inst.def.Name, stateKey)

	var features []Feature
	switch inst.def.Kind {
	case "aggregate", "slope":
		fs, err := r.applyWindowOperator(inst, blob, env, watermark)
		if err != nil {
			return nil, err
		}
		features = fs
	case "missing_heartbeat":
		fs, err := r.applyHeartbeatOperator(inst, blob, env, watermark)
		if err != nil {
			return nil, err
		}
		features = fs
	default:
		return nil, fmt.Errorf("unsupported operator kind %q", inst.def.Kind)
	}

	r.setBlob(ps, inst.def.Name, stateKey, blob)
	return features, nil
}

func (r *OperatorRuntime) getBlob(ps *PartitionState, operatorID, stateKey string) *OperatorStateBlob {
	if ps.OperatorStates == nil {
		ps.OperatorStates = make(map[string]map[string]*OperatorStateBlob)
	}
	ops, ok := ps.OperatorStates[operatorID]
	if !ok {
		ops = make(map[string]*OperatorStateBlob)
		ps.OperatorStates[operatorID] = ops
	}
	blob, ok := ops[stateKey]
	if !ok {
		blob = &OperatorStateBlob{}
		ops[stateKey] = blob
	}
	return blob
}

func (r *OperatorRuntime) setBlob(ps *PartitionState, operatorID, stateKey string, blob *OperatorStateBlob) {
	if ps.OperatorStates == nil {
		ps.OperatorStates = make(map[string]map[string]*OperatorStateBlob)
	}
	ops, ok := ps.OperatorStates[operatorID]
	if !ok {
		ops = make(map[string]*OperatorStateBlob)
		ps.OperatorStates[operatorID] = ops
	}
	ops[stateKey] = blob
}

func (r *OperatorRuntime) applyWindowOperator(inst *operatorInstance, blob *OperatorStateBlob, env contractsv1.Envelope, watermark time.Time) ([]Feature, error) {
	if blob.Window == nil {
		blob.Window = &WindowState{}
	}
	ws := blob.Window

	value, ok := extractValue(inst.def.Field, env)
	if !ok {
		return nil, nil
	}

	ws.Samples = append(ws.Samples, Sample{
		EventID:   env.ID,
		EventTime: env.EventTime,
		Value:     value,
	})

	// Evict samples outside the window.
	cutoff := watermark.Add(-inst.window.size)
	filtered := ws.Samples[:0]
	for _, s := range ws.Samples {
		if !s.EventTime.Before(cutoff) {
			filtered = append(filtered, s)
		}
	}
	ws.Samples = filtered

	// Sort samples by event time for deterministic output.
	sort.SliceStable(ws.Samples, func(i, j int) bool {
		if ws.Samples[i].EventTime.Equal(ws.Samples[j].EventTime) {
			return ws.Samples[i].EventID < ws.Samples[j].EventID
		}
		return ws.Samples[i].EventTime.Before(ws.Samples[j].EventTime)
	})

	agg, err := computeAggregate(inst.def.Aggregate, ws.Samples, inst.def.Unit)
	if err != nil {
		return nil, err
	}

	windowStart := watermark.Add(-inst.window.size)
	windowEnd := watermark

	var features []Feature
	emit := false
	completeness := string(CompletenessProvisional)

	switch inst.window.emit {
	case "on_update":
		emit = len(ws.Samples) > 0
	case "early_and_close":
		emit = len(ws.Samples) > 0
		completeness = string(CompletenessProvisional)
	case "on_close":
		emit = false // Only emitted by timer/watermark close.
	}

	if emit && len(ws.Samples) > 0 {
		feature := Feature{
			FeatureID:     r.idGen.New(ids.PrefixEvent),
			OperatorID:    inst.def.Name,
			OutputName:    inst.def.Output,
			TenantID:      env.TenantID,
			EntityType:    env.Entity.Type,
			EntityID:      env.Entity.ID,
			PartitionID:   env.PartitionID(0),
			WindowStart:   windowStart,
			WindowEnd:     windowEnd,
			Value:         agg,
			Unit:          inst.def.Unit,
			EventTime:     env.EventTime,
			Watermark:     watermark,
			InputEventIDs: eventIDs(ws.Samples),
			Completeness:  completeness,
			Traceparent:   env.Traceparent,
			Tracestate:    env.Tracestate,
		}
		features = append(features, feature)
		ws.LastEmit = watermark
	}

	return features, nil
}

func (r *OperatorRuntime) applyHeartbeatOperator(inst *operatorInstance, blob *OperatorStateBlob, env contractsv1.Envelope, watermark time.Time) ([]Feature, error) {
	if blob.Heartbeat == nil {
		blob.Heartbeat = &HeartbeatState{}
	}
	hs := blob.Heartbeat
	hs.LastEventTime = &env.EventTime
	hs.LastEventID = env.ID

	duration, err := parseDuration(inst.def.Duration)
	if err != nil {
		return nil, fmt.Errorf("heartbeat duration: %w", err)
	}

	missing := false
	if hs.LastEventTime != nil {
		missing = watermark.Sub(*hs.LastEventTime) > duration
	}

	feature := Feature{
		FeatureID:     r.idGen.New(ids.PrefixEvent),
		OperatorID:    inst.def.Name,
		OutputName:    inst.def.Output,
		TenantID:      env.TenantID,
		EntityType:    env.Entity.Type,
		EntityID:      env.Entity.ID,
		PartitionID:   env.PartitionID(0),
		WindowStart:   env.EventTime,
		WindowEnd:     watermark,
		Value:         missing,
		EventTime:     env.EventTime,
		Watermark:     watermark,
		InputEventIDs: []string{env.ID},
		Completeness:  string(CompletenessOnTime),
		Traceparent:   env.Traceparent,
		Tracestate:    env.Tracestate,
	}
	return []Feature{feature}, nil
}

func extractValue(field string, env contractsv1.Envelope) (float64, bool) {
	if field == "" {
		return 0, false
	}
	parts := strings.Split(field, ".")
	if len(parts) != 2 || parts[0] != "data" {
		return 0, false
	}
	v, ok := env.Data[parts[1]]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func computeAggregate(agg string, samples []Sample, unit string) (float64, error) {
	if len(samples) == 0 {
		return 0, nil
	}
	switch agg {
	case "mean":
		var sum float64
		for _, s := range samples {
			sum += s.Value
		}
		return sum / float64(len(samples)), nil
	case "rms":
		var sumSquares float64
		for _, s := range samples {
			sumSquares += s.Value * s.Value
		}
		return math.Sqrt(sumSquares / float64(len(samples))), nil
	case "slope":
		return linearSlope(samples, unit), nil
	case "count":
		return float64(len(samples)), nil
	case "sum":
		var sum float64
		for _, s := range samples {
			sum += s.Value
		}
		return sum, nil
	case "min":
		m := samples[0].Value
		for _, s := range samples {
			if s.Value < m {
				m = s.Value
			}
		}
		return m, nil
	case "max":
		m := samples[0].Value
		for _, s := range samples {
			if s.Value > m {
				m = s.Value
			}
		}
		return m, nil
	default:
		return 0, fmt.Errorf("unsupported aggregate %q", agg)
	}
}

func linearSlope(samples []Sample, unit string) float64 {
	if len(samples) < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	start := samples[0].EventTime
	for _, s := range samples {
		x := s.EventTime.Sub(start).Hours()
		sumX += x
		sumY += s.Value
		sumXY += x * s.Value
		sumXX += x * x
	}
	n := float64(len(samples))
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0
	}
	slope := (n*sumXY - sumX*sumY) / denom
	if strings.Contains(unit, "per_second") || strings.Contains(unit, "_per_s") {
		return slope / 3600
	}
	return slope
}

func eventIDs(samples []Sample) []string {
	ids := make([]string, len(samples))
	for i, s := range samples {
		ids[i] = s.EventID
	}
	return ids
}

// ApplyTimer fires due timers and emits any resulting features. In Phase 2 this
// is used primarily for missing-heartbeat detection.
func (r *OperatorRuntime) ApplyTimer(ctx context.Context, ps *PartitionState, watermark time.Time) ([]Feature, *PartitionState, error) {
	_ = ctx
	if ps == nil {
		return nil, ps, nil
	}

	var features []Feature
	for _, inst := range r.byInput {
		for _, op := range inst {
			if op.def.Kind != "missing_heartbeat" {
				continue
			}
			fs, err := r.applyHeartbeatTimer(op, ps, watermark)
			if err != nil {
				return nil, ps, err
			}
			features = append(features, fs...)
		}
	}
	return features, ps, nil
}

func (r *OperatorRuntime) applyHeartbeatTimer(inst *operatorInstance, ps *PartitionState, watermark time.Time) ([]Feature, error) {
	duration, err := parseDuration(inst.def.Duration)
	if err != nil {
		return nil, fmt.Errorf("heartbeat duration: %w", err)
	}

	var features []Feature
	for stateKey, blob := range ps.OperatorStates[inst.def.Name] {
		if blob.Heartbeat == nil || blob.Heartbeat.LastEventTime == nil {
			continue
		}
		missing := watermark.Sub(*blob.Heartbeat.LastEventTime) > duration
		if !missing {
			continue
		}
		// Determine entity type/id from stateKey. For Phase 2 entity type is
		// known from the spec input.
		entityType := r.entityTypeForOperator(inst.def.Name)
		features = append(features, Feature{
			FeatureID:     r.idGen.New(ids.PrefixEvent),
			OperatorID:    inst.def.Name,
			OutputName:    inst.def.Output,
			TenantID:      contractsv1.TenantID,
			EntityType:    entityType,
			EntityID:      stateKey,
			PartitionID:   0,
			WindowStart:   *blob.Heartbeat.LastEventTime,
			WindowEnd:     watermark,
			Value:         true,
			EventTime:     watermark,
			Watermark:     watermark,
			InputEventIDs: []string{blob.Heartbeat.LastEventID},
			Completeness:  string(CompletenessOnTime),
		})
	}
	return features, nil
}

func (r *OperatorRuntime) entityTypeForOperator(operatorID string) string {
	for _, op := range r.spec.Operators {
		if op.Name == operatorID {
			for _, in := range op.Inputs {
				for _, specIn := range r.spec.Inputs {
					if specIn.Name == in {
						return specIn.EntityType
					}
				}
			}
		}
	}
	return ""
}
