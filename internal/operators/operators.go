package operators

import (
	"context"
	"fmt"
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
	emit := w.Emit
	if emit == "" {
		emit = "on_close"
	}
	switch emit {
	case "on_update", "on_close", "early_and_close":
	default:
		return nil, fmt.Errorf("unsupported emit mode %q", emit)
	}
	switch w.Kind {
	case "tumbling":
		return tumblingWindow(w, emit)
	case "sliding":
		return slidingWindow(w, emit)
	default:
		return nil, fmt.Errorf("unsupported window kind %q", w.Kind)
	}
}

// tumblingWindow requires a positive size and no slide.
func tumblingWindow(w spec.Window, emit string) (*windowConfig, error) {
	if w.Slide != "" {
		return nil, fmt.Errorf("tumbling windows do not support slide")
	}
	size, err := parseDuration(w.Size)
	if err != nil {
		return nil, fmt.Errorf("tumbling size: %w", err)
	}
	if size <= 0 {
		return nil, fmt.Errorf("tumbling size must be positive")
	}
	return &windowConfig{emit: emit, size: size}, nil
}

// slidingWindow requires a positive size and a positive slide no longer than
// the size.
func slidingWindow(w spec.Window, emit string) (*windowConfig, error) {
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
	return &windowConfig{emit: emit, size: size, slide: slide}, nil
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
