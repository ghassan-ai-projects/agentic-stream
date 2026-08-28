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
	Runtime   *RuntimeState   `json:"runtime,omitempty"`
}

const maxSeenBootIDs = 64

// ApplyEvent processes one event against all operators that consume its input.
func (r *OperatorRuntime) ApplyEvent(ctx context.Context, ps *PartitionState, env contractsv1.Envelope, watermark time.Time) ([]Feature, *PartitionState, error) {
	processingTime := env.IngestedAt
	return r.ApplyEventAt(ctx, ps, env, watermark, processingTime)
}

// ApplyEventAt applies one event using processingTime supplied by the runtime
// clock. Producer timestamps are evidence, not runtime scheduling authority.
func (r *OperatorRuntime) ApplyEventAt(ctx context.Context, ps *PartitionState, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, *PartitionState, error) {
	if ps == nil {
		ps = &PartitionState{OperatorStates: make(map[string]map[string]*OperatorStateBlob)}
	}

	inputName, ok := r.inputForEvent(env)
	if !ok {
		return nil, ps, nil
	}
	if !r.qualityAdmitsBoot(inputName, env) {
		return nil, ps, nil
	}
	if !r.admitBoot(ps, env) {
		return nil, ps, nil
	}

	var features []Feature
	for _, inst := range r.byInput[inputName] {
		if len(inst.def.Inputs) > 0 && inst.def.Inputs[0] != inputName {
			continue // Only direct inputs supported for now.
		}
		fs, err := r.applyOperator(ctx, ps, inst, env, watermark, processingTime)
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

func (r *OperatorRuntime) applyOperator(ctx context.Context, ps *PartitionState, inst *operatorInstance, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, error) {
	stateKey := operatorStateKey(env)
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
		fs, err := r.applyHeartbeatOperator(inst, blob, env, watermark, processingTime)
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
	value, ok := extractValue(inst.def.Field, env)
	if !ok {
		return nil, nil
	}

	if blob.Window == nil {
		blob.Window = &WindowState{}
	}
	ws := blob.Window
	bootID := deviceBootID(env)
	if ws.BootID == "" {
		ws.BootID = bootID
	}
	corrected, err := r.isLateWindowCorrection(inst.window.size, env.EventTime, watermark, ws.LastEmit)
	if err != nil {
		return nil, err
	}

	ws.Samples = append(ws.Samples, Sample{
		EventID:   env.ID,
		EventTime: env.EventTime,
		Value:     value,
		BootID:    bootID,
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
	if corrected {
		completeness = string(CompletenessCorrected)
	}

	if emit && len(ws.Samples) > 0 {
		feature := Feature{
			FeatureID:     r.idGen.New(ids.PrefixEvent),
			OperatorID:    inst.def.Name,
			OutputName:    inst.def.Output,
			TenantID:      env.TenantID,
			EntityType:    env.Entity.Type,
			EntityID:      env.Entity.ID,
			StateKey:      operatorStateKey(env),
			BootID:        bootID,
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

func (r *OperatorRuntime) isLateWindowCorrection(windowSize time.Duration, eventTime, watermark, lastEmit time.Time) (bool, error) {
	if r.spec.Time.LatePolicy != "correct" && r.spec.Time.LatePolicy != "correct_and_reconsider" {
		return false, nil
	}
	if lastEmit.IsZero() || !eventTime.Before(watermark) || !eventTime.Before(lastEmit) || eventTime.Before(lastEmit.Add(-windowSize)) {
		return false, nil
	}
	if r.spec.Time.AllowedLateness == "" {
		return false, nil
	}
	allowedLateness, err := parseDuration(r.spec.Time.AllowedLateness)
	if err != nil {
		return false, fmt.Errorf("allowed lateness: %w", err)
	}
	return watermark.Sub(eventTime) <= allowedLateness, nil
}

func (r *OperatorRuntime) applyHeartbeatOperator(inst *operatorInstance, blob *OperatorStateBlob, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, error) {
	if !sampleQualityValid(env) {
		return nil, nil
	}

	if blob.Heartbeat == nil {
		blob.Heartbeat = &HeartbeatState{}
	}
	hs := blob.Heartbeat
	bootID := deviceBootID(env)
	if bootID != "" && hs.BootID != bootID {
		*hs = HeartbeatState{BootID: bootID}
	} else if hs.BootID == "" {
		hs.BootID = bootID
	}
	hs.LastEventTime = &env.EventTime
	hs.LastEventID = env.ID
	hs.Traceparent = env.Traceparent
	hs.Tracestate = env.Tracestate
	if processingTime.IsZero() {
		processingTime = env.EventTime
	}
	hs.LastProcessingTime = &processingTime

	duration, err := parseDuration(inst.def.Duration)
	if err != nil {
		return nil, fmt.Errorf("heartbeat duration: %w", err)
	}

	missing := false
	if hs.LastEventTime != nil {
		missing = watermark.Sub(*hs.LastEventTime) >= duration
	}
	eventTime := env.EventTime
	if missing {
		eventTime = processingTime
	}

	feature := Feature{
		FeatureID:     r.idGen.New(ids.PrefixEvent),
		OperatorID:    inst.def.Name,
		OutputName:    inst.def.Output,
		TenantID:      env.TenantID,
		EntityType:    env.Entity.Type,
		EntityID:      env.Entity.ID,
		StateKey:      operatorStateKey(env),
		BootID:        bootID,
		PartitionID:   env.PartitionID(0),
		WindowStart:   env.EventTime,
		WindowEnd:     watermark,
		Value:         missing,
		EventTime:     eventTime,
		Watermark:     watermark,
		InputEventIDs: []string{env.ID},
		Completeness:  string(CompletenessOnTime),
		Traceparent:   env.Traceparent,
		Tracestate:    env.Tracestate,
	}
	if missing {
		feature.Completeness = string(CompletenessUncertain)
	}
	return []Feature{feature}, nil
}

func extractValue(field string, env contractsv1.Envelope) (float64, bool) {
	if field == "" || !numericObservationQualityValid(env) {
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
		return finiteValue(x)
	case float32:
		return finiteValue(float64(x))
	case int:
		return finiteValue(float64(x))
	case int64:
		return finiteValue(float64(x))
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		return finiteValue(f)
	}
	return 0, false
}

func finiteValue(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// sampleQualityValid accepts legacy payloads that do not carry a quality
// field, but every supplied sample quality must explicitly be valid. Quality
// flags on the envelope can independently invalidate a numeric sample.
func sampleQualityValid(env contractsv1.Envelope) bool {
	if raw, exists := env.Data["quality"]; exists {
		quality, ok := raw.(string)
		if !ok || quality != "valid" {
			return false
		}
	}

	for _, flag := range env.Quality {
		switch strings.ToLower(strings.TrimSpace(flag.Code)) {
		case "warming", "invalid", "disconnected", "rail_high", "rail_low":
			return false
		default:
			// Unknown quality flags are not safe to interpret optimistically.
			return false
		}
	}
	return true
}

// numericObservationQualityValid applies the strict physical-observation
// contract to the thermal wire family. The generic operator tests and legacy
// logical inputs retain their existing quality compatibility until their
// schemas opt into the physical provenance contract.
func numericObservationQualityValid(env contractsv1.Envelope) bool {
	if !strings.HasPrefix(env.Type, "zone.") {
		return sampleQualityValid(env)
	}
	if !sampleQualityValid(env) || deviceBootID(env) == "" {
		return false
	}
	quality, ok := env.Data["quality"].(string)
	return ok && quality == "valid"
}

func (r *OperatorRuntime) qualityAdmitsBoot(inputName string, env contractsv1.Envelope) bool {
	for _, inst := range r.byInput[inputName] {
		if inst.def.Kind == "missing_heartbeat" {
			return sampleQualityValid(env)
		}
		if inst.def.Kind == "aggregate" || inst.def.Kind == "slope" {
			return numericObservationQualityValid(env)
		}
	}
	return true
}

func deviceBootID(env contractsv1.Envelope) string {
	bootID, ok := env.Data["boot_id"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(bootID)
}

func operatorStateKey(env contractsv1.Envelope) string {
	bootID := deviceBootID(env)
	if bootID == "" {
		return env.Entity.ID
	}
	// Device sequence and monotonic time are meaningful only within a boot.
	// Keep explicit boots in separate durable state keys so a delayed event from
	// an older boot cannot reset or contaminate the current boot's window.
	return env.Entity.ID + "\x1f" + bootID
}

func entityIDFromStateKey(stateKey string) string {
	if entityID, _, ok := strings.Cut(stateKey, "\x1f"); ok {
		return entityID
	}
	return stateKey
}

func bootIDFromStateKey(stateKey string) string {
	_, bootID, ok := strings.Cut(stateKey, "\x1f")
	if !ok {
		return ""
	}
	return bootID
}

// admitBoot fences delayed evidence from a prior device boot. Missing boot
// identity remains compatible only until the first identified boot is seen;
// thereafter it cannot be used to mutate physical numeric state.
func (r *OperatorRuntime) admitBoot(ps *PartitionState, env contractsv1.Envelope) bool {
	stateKey := env.Entity.ID
	meta := r.getBlob(ps, RuntimeOperatorID, stateKey)
	if meta.Runtime == nil {
		meta.Runtime = &RuntimeState{}
	}
	runtimeState := meta.Runtime
	bootID := deviceBootID(env)
	if bootID == "" {
		return runtimeState.CurrentBootID == ""
	}
	if runtimeState.CurrentBootID == bootID {
		return true
	}
	for _, seen := range runtimeState.SeenBootIDs {
		if seen == bootID {
			return false
		}
	}
	if len(runtimeState.SeenBootIDs) >= maxSeenBootIDs {
		// A bounded history cannot safely distinguish an evicted old boot from
		// a new one. Fail closed rather than allowing stale evidence to revive.
		return false
	}
	for operatorID, states := range ps.OperatorStates {
		if operatorID == RuntimeOperatorID {
			continue
		}
		for stateKey := range states {
			if stateKey == env.Entity.ID || strings.HasPrefix(stateKey, env.Entity.ID+"\x1f") {
				// Only the active boot's state is useful after admission. The
				// tombstone list above still prevents a retired boot from being
				// admitted again.
				delete(states, stateKey)
			}
		}
	}
	runtimeState.CurrentBootID = bootID
	runtimeState.SeenBootIDs = append(runtimeState.SeenBootIDs, bootID)
	return true
}

func (r *OperatorRuntime) isActiveBoot(ps *PartitionState, stateKey string) bool {
	bootID := bootIDFromStateKey(stateKey)
	meta := ps.OperatorStates[RuntimeOperatorID][entityIDFromStateKey(stateKey)]
	if bootID == "" {
		return meta == nil || meta.Runtime == nil || meta.Runtime.CurrentBootID == ""
	}
	return meta == nil || meta.Runtime == nil || meta.Runtime.CurrentBootID == "" || meta.Runtime.CurrentBootID == bootID
}

// IsTimerStateActive reports whether a persisted timer belongs to the current
// boot admission state. The engine uses this to retire stale timers instead of
// treating their intentional suppression as a processing failure.
func (r *OperatorRuntime) IsTimerStateActive(ps *PartitionState, stateKey string) bool {
	if ps == nil {
		return false
	}
	return r.isActiveBoot(ps, stateKey)
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
	case "latest":
		// Samples are sorted by event time and then event ID before this
		// function is called. The final sample is therefore deterministic even
		// when events arrive out of order or share an event timestamp.
		return samples[len(samples)-1].Value, nil
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
func (r *OperatorRuntime) ApplyTimer(ctx context.Context, ps *PartitionState, watermark, processingTime time.Time) ([]Feature, *PartitionState, error) {
	_ = ctx
	if ps == nil {
		return nil, ps, nil
	}

	var features []Feature
	var instances []*operatorInstance
	for _, inputInstances := range r.byInput {
		instances = append(instances, inputInstances...)
	}
	sort.SliceStable(instances, func(i, j int) bool {
		return instances[i].def.Name < instances[j].def.Name
	})
	seen := make(map[string]struct{}, len(instances))
	for _, op := range instances {
		if _, ok := seen[op.def.Name]; ok {
			continue
		}
		seen[op.def.Name] = struct{}{}
		if op.def.Kind != "missing_heartbeat" {
			continue
		}
		fs, err := r.applyHeartbeatTimer(op, ps, watermark, processingTime)
		if err != nil {
			return nil, ps, err
		}
		features = append(features, fs...)
	}
	return features, ps, nil
}

func (r *OperatorRuntime) applyHeartbeatTimer(inst *operatorInstance, ps *PartitionState, watermark, processingTime time.Time) ([]Feature, error) {
	duration, err := parseDuration(inst.def.Duration)
	if err != nil {
		return nil, fmt.Errorf("heartbeat duration: %w", err)
	}

	var features []Feature
	keys := make([]string, 0, len(ps.OperatorStates[inst.def.Name]))
	for stateKey := range ps.OperatorStates[inst.def.Name] {
		keys = append(keys, stateKey)
	}
	sort.Strings(keys)
	for _, stateKey := range keys {
		if !r.isActiveBoot(ps, stateKey) {
			continue
		}
		blob := ps.OperatorStates[inst.def.Name][stateKey]
		if blob.Heartbeat == nil || blob.Heartbeat.LastEventTime == nil {
			continue
		}
		lastProcessingTime := blob.Heartbeat.LastProcessingTime
		if lastProcessingTime == nil {
			lastProcessingTime = blob.Heartbeat.LastEventTime
		}
		missing := processingTime.Sub(*lastProcessingTime) >= duration
		if !missing {
			continue
		}
		// Determine entity type/id from stateKey. For Phase 2 entity type is
		// known from the spec input.
		entityType := r.entityTypeForOperator(inst.def.Name)
		features = append(features, Feature{
			FeatureID:         r.idGen.New(ids.PrefixEvent),
			OperatorID:        inst.def.Name,
			OutputName:        inst.def.Output,
			TenantID:          contractsv1.TenantID,
			EntityType:        entityType,
			EntityID:          entityIDFromStateKey(stateKey),
			StateKey:          stateKey,
			BootID:            blob.Heartbeat.BootID,
			PartitionID:       0,
			WindowStart:       *blob.Heartbeat.LastEventTime,
			WindowEnd:         processingTime,
			Value:             true,
			EventTime:         processingTime,
			Watermark:         watermark,
			InputEventIDs:     []string{blob.Heartbeat.LastEventID},
			Completeness:      string(CompletenessUncertain),
			TraceContinuation: true,
			Traceparent:       blob.Heartbeat.Traceparent,
			Tracestate:        blob.Heartbeat.Tracestate,
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
