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
	windowConfigs, err := buildWindowConfigs(compiled.Windows)
	if err != nil {
		return nil, err
	}
	byInput, err := indexOperatorsByInput(compiled.Operators, windowConfigs)
	if err != nil {
		return nil, err
	}
	return &OperatorRuntime{deploymentID: deploymentID, spec: compiled, idGen: idGen, byInput: byInput}, nil
}

func buildWindowConfigs(windows []spec.Window) (map[string]*windowConfig, error) {
	windowConfigs := make(map[string]*windowConfig, len(windows))
	for _, w := range windows {
		cfg, err := newWindowConfig(w)
		if err != nil {
			return nil, fmt.Errorf("window %s: %w", w.Name, err)
		}
		windowConfigs[w.Name] = cfg
	}
	return windowConfigs, nil
}

// indexOperatorsByInput binds each operator to its window and lists it under
// every input it consumes.
func indexOperatorsByInput(operators []spec.Operator, windowConfigs map[string]*windowConfig) (map[string][]*operatorInstance, error) {
	byInput := make(map[string][]*operatorInstance)
	for _, op := range operators {
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
	return byInput, nil
}

func newWindowConfig(w spec.Window) (*windowConfig, error) {
	emit, err := emitMode(w.Emit)
	if err != nil {
		return nil, err
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

// emitMode defaults an empty mode to on_close and rejects unknown modes.
func emitMode(emit string) (string, error) {
	switch emit {
	case "":
		return "on_close", nil
	case "on_update", "on_close", "early_and_close":
		return emit, nil
	default:
		return "", fmt.Errorf("unsupported emit mode %q", emit)
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
	if err := checkSlide(size, slide); err != nil {
		return nil, err
	}
	return &windowConfig{emit: emit, size: size, slide: slide}, nil
}

func checkSlide(size, slide time.Duration) error {
	if size <= 0 {
		return fmt.Errorf("sliding size must be positive")
	}
	if slide <= 0 {
		return fmt.Errorf("sliding slide must be positive")
	}
	if slide > size {
		return fmt.Errorf("sliding slide %s exceeds size %s", slide, size)
	}
	return nil
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
	features, err := r.applyInput(ctx, ps, inputName, env, watermark, processingTime)
	if err != nil {
		return nil, ps, err
	}
	return features, ps, nil
}

// applyInput runs every operator fed directly by the input that admits the
// event and its device boot.
func (r *OperatorRuntime) applyInput(ctx context.Context, ps *PartitionState, inputName string, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, error) {
	var features []Feature
	for _, inst := range r.byInput[inputName] {
		if !r.operatorApplies(ps, inst, inputName, env) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("apply event canceled: %w", err)
		}
		fs, err := r.applyOperator(ctx, ps, inst, env, watermark, processingTime)
		if err != nil {
			return nil, err
		}
		features = append(features, fs...)
	}
	return features, nil
}

// operatorApplies requires a direct input (only direct inputs are supported
// for now), an admitted event and an admitted device boot, in that order.
func (r *OperatorRuntime) operatorApplies(ps *PartitionState, inst *operatorInstance, inputName string, env contractsv1.Envelope) bool {
	if len(inst.def.Inputs) > 0 && inst.def.Inputs[0] != inputName {
		return false
	}
	return r.operatorAdmitsEvent(inst, env) && r.admitBoot(ps, env)
}

func (r *OperatorRuntime) inputForEvent(env contractsv1.Envelope) (string, bool) {
	for _, in := range r.spec.Inputs {
		if in.EventType == env.Type {
			return in.Name, true
		}
	}
	return "", false
}

func (r *OperatorRuntime) applyOperator(_ context.Context, ps *PartitionState, inst *operatorInstance, env contractsv1.Envelope, watermark, processingTime time.Time) ([]Feature, error) {
	blob := r.getBlob(ps, inst.def.Name, operatorStateKey(env))
	switch inst.def.Kind {
	case "aggregate", "slope":
		return r.applyWindowOperator(inst, blob, env, watermark)
	case "missing_heartbeat":
		return r.applyHeartbeatOperator(inst, blob, env, watermark, processingTime)
	default:
		return nil, fmt.Errorf("unsupported operator kind %q", inst.def.Kind)
	}
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
