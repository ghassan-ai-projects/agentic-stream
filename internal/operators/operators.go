package operators

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/duration"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
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
	windowConfigs := make(map[string]*windowConfig, len(compiled.Windows))
	for _, w := range compiled.Windows {
		cfg, err := newWindowConfig(w)
		if err != nil {
			return nil, fmt.Errorf("window %s: %w", w.Name, err)
		}
		windowConfigs[w.Name] = cfg
	}

	byInput := make(map[string][]*operatorInstance)
	for i := range compiled.Operators {
		op := compiled.Operators[i]
		inst := &operatorInstance{def: op}
		if op.Window != "" {
			cfg, ok := windowConfigs[op.Window]
			if !ok {
				return nil, fmt.Errorf("operator %s references unknown window %s", op.Name, op.Window)
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
	switch cfg.emit {
	case "on_update", "on_close", "early_and_close":
	default:
		return nil, fmt.Errorf("unsupported emit mode %q", cfg.emit)
	}

	switch w.Kind {
	case "tumbling":
		if w.Slide != "" {
			return nil, fmt.Errorf("tumbling windows do not support slide")
		}
		d, err := parseDuration(w.Size)
		if err != nil {
			return nil, fmt.Errorf("tumbling size: %w", err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("tumbling size must be positive")
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
		if size <= 0 {
			return nil, fmt.Errorf("sliding size must be positive")
		}
		if slide <= 0 {
			return nil, fmt.Errorf("sliding slide must be positive")
		}
		if slide > size {
			return nil, fmt.Errorf("sliding slide %s exceeds size %s", slide, size)
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

// ApplyEventAt applies one event using processingTime supplied by the runtime
// clock. Producer timestamps are evidence, not runtime scheduling authority.
func (r *OperatorRuntime) ApplyEventAt(ctx context.Context, ps *PartitionState, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, *PartitionState, error) {
	if err := ctx.Err(); err != nil {
		return nil, ps, fmt.Errorf("apply event canceled: %w", err)
	}
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
		if !r.operatorAdmitsEvent(inst, env) || !r.admitBoot(ps, env) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, ps, fmt.Errorf("apply event canceled: %w", err)
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

func (r *OperatorRuntime) extractValue(inst *operatorInstance, env contractsv1.Envelope) (float64, bool) {
	if inst.def.Field == "" || !r.numericObservationQualityValid(inst, env) {
		return 0, false
	}
	parts := strings.Split(inst.def.Field, ".")
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

func (r *OperatorRuntime) operatorAdmitsEvent(inst *operatorInstance, env contractsv1.Envelope) bool {
	switch inst.def.Kind {
	case "missing_heartbeat":
		return sampleQualityValid(env)
	case "aggregate", "slope":
		return r.numericObservationQualityValid(inst, env)
	default:
		return true
	}
}

// numericObservationQualityValid applies the provenance contract declared by
// the input's registered schema. A schema that exposes both quality and boot
// identity requires a valid quality and an identified boot; no event family is
// special-cased in the runtime.
func (r *OperatorRuntime) numericObservationQualityValid(inst *operatorInstance, env contractsv1.Envelope) bool {
	if !sampleQualityValid(env) {
		return false
	}
	if !r.inputRequiresBootIdentity(inst) {
		return true
	}
	return deviceBootID(env) != ""
}

func (r *OperatorRuntime) inputRequiresBootIdentity(inst *operatorInstance) bool {
	for _, inputName := range inst.def.Inputs {
		for _, input := range r.spec.Inputs {
			if input.Name != inputName || input.SchemaRef == "" {
				continue
			}
			definition, ok := eventschema.Lookup(input.SchemaRef)
			if !ok {
				return false
			}
			_, hasQuality := definition.Fields["quality"]
			_, hasBootID := definition.Fields["boot_id"]
			return hasQuality && hasBootID
		}
	}
	return false
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
