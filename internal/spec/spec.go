package spec

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

// NewCELEnv creates a restricted CEL environment for SituationSpec expressions.
// It allows only deterministic functions and rejects sources of non-determinism
// such as timestamps, randomness, and external calls.
func NewCELEnv() (*cel.Env, error) {
	return domain.NewCELEnv()
}

// CELBool converts an evaluated SituationSpec condition to a Go bool,
// rejecting any non-boolean result.
func CELBool(out ref.Val) (bool, error) {
	return domain.CELBool(out)
}

// ParseDuration converts a duration string such as "15m", "6h", or "30d" into a
// time.Duration. It accepts the same base units as time.ParseDuration plus
// days ("d").
func ParseDuration(s string) (time.Duration, error) {
	return domain.ParseDuration(s)
}

// EventSchema identifies a schema bound to one normalized event type.
type EventSchema = domain.EventSchema

// EventSchemas is an immutable event-schema registry: the schemas compiled
// into the binary, plus any loaded from a directory. A compiled spec keeps the
// registry it was compiled against; CompiledSpec.EventSchema looks a ref up in
// it, falling back to the built-in schemas for a spec loaded from a
// deployment.
type EventSchemas = domain.EventSchemas

// BuiltinEventSchemas returns the event schemas compiled into the binary.
func BuiltinEventSchemas() *EventSchemas {
	return domain.BuiltinEventSchemas()
}

// LoadEventSchemas reads every *.json file of dir, in name order, in the
// format of the built-in registry (a map from "<event_type>/<schema_version>"
// to the schema), and returns the built-in schemas merged with them. A ref
// that is already registered must have an identical definition, so a
// directory can add schemas but never change one.
//
// Every schema is checked as it loads, so a bad directory fails at start:
// each field has a plain name and path, a type of number, integer, string or
// boolean (empty means number), an enum only on a string field, and the
// JSON Schema the entry produces must compile. A document must declare at
// least one schema and may not repeat a key. Only regular files are read
// (a link, directory or device named *.json is refused, as is *.JSON); at
// most 64 files of at most 1 MiB each.
func LoadEventSchemas(dir string) (*EventSchemas, error) {
	schemas, err := app.LoadEventSchemas(dir)
	if err != nil {
		return nil, fmt.Errorf("load event schemas from %s: %w", dir, err)
	}
	return schemas, nil
}

// CompiledSpec is the immutable result of compiling a SituationSpec.
type CompiledSpec = domain.CompiledSpec

var DeltaKeys = domain.DeltaKeys

// Input declares an accepted event schema and how it maps to the runtime.
type Input = domain.Input

// TimePolicy configures event-time semantics.
type TimePolicy = domain.TimePolicy

// Window defines a named time or count window.
type Window = domain.Window

// Operator defines a deterministic computation over inputs.
type Operator = domain.Operator

// Phase is a state in the Situation lifecycle.
type Phase = domain.Phase

// Transition defines a phase change.
type Transition = domain.Transition

// Reducer defines how a Situation field is derived from operator outputs.
type Reducer = domain.Reducer

// Occurrence controls when a new Situation is opened.
type Occurrence = domain.Occurrence

// Situation configures the central semantic aggregate.
type Situation = domain.Situation

// Trigger configures when cognition is invoked.
type Trigger = domain.Trigger

// SkillRef is one digest-pinned skill the episode may render into its frame —
// P5/§B6-B9: the worker resolves the text ONLY from the operator-approved
// directory and requires the tree digest to match.
type SkillRef = domain.SkillRef

// Executor configures the episode runtime.
type Executor = domain.Executor

// Dispatch policies an Executor may declare, and the trigger lanes a Trigger
// may declare. The spec owns this vocabulary; consumers compare against these.
const (
	DispatchShadow = domain.DispatchShadow
	DispatchActive = domain.DispatchActive
	LaneFast       = domain.LaneFast
	LaneDeep       = domain.LaneDeep
)

// EffectiveDispatchPolicy is the declared policy, or shadow when none was
// declared: nothing enters action governance unless the spec declared active.
func EffectiveDispatchPolicy(policy string) string {
	return domain.EffectiveDispatchPolicy(policy)
}

// Budget caps episode resource usage.
type Budget = domain.Budget

// Cognition groups trigger and executor configuration.
type Cognition = domain.Cognition

// Intent declares one action type and its authority: the EXACT risk class,
// the parameter schema, the operator-authored presets, the model-writable
// fields, and the policy/rate-limit/compensation metadata. P4: these compile
// into the canonical intent catalog the worker verifies and the validator
// enforces independently (B9/B10).
type Intent = domain.Intent

// Actions configures allowed intents.
type Actions = domain.Actions

// CompileOption adjusts one compilation.
type CompileOption = app.CompileOption

// WithEventSchemas compiles against schemas instead of the built-in event
// schemas. A nil registry means the built-in one.
func WithEventSchemas(schemas *EventSchemas) CompileOption {
	return app.WithEventSchemas(schemas)
}

// CompileFile reads a SituationSpec from path and compiles it.
func CompileFile(ctx context.Context, path string, options ...CompileOption) (*CompiledSpec, error) {
	compiled, err := app.CompileFile(ctx, path, options...)
	if err != nil {
		return nil, fmt.Errorf("compile spec %s: %w", path, err)
	}
	return compiled, nil
}

// SaveDeployment persists a compiled spec as an active deployment record. It is
// idempotent for the same deployment.
func SaveDeployment(ctx context.Context, db *storage.DB, tenantID string, compiled *CompiledSpec) error {
	if err := store.SaveDeployment(ctx, db, tenantID, compiled); err != nil {
		return fmt.Errorf("save deployment: %w", err)
	}
	return nil
}

// FieldDerivation explains one Situation field from the spec: the reducer
// that writes it and the operator that computes the reducer's input.
type FieldDerivation = domain.FieldDerivation

// LoadDeployment reads the compiled spec a deployment stored.
func LoadDeployment(ctx context.Context, db *storage.DB, deploymentID string) (*CompiledSpec, error) {
	compiled, err := store.LoadDeployment(ctx, db, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("load deployment: %w", err)
	}
	return compiled, nil
}
