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

// Lookup returns a registered built-in definition.
func LookupEventSchema(ref string) (domain.EventSchema, bool) {
	return domain.LookupEventSchema(ref)
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

// CompileFile reads a SituationSpec from path and compiles it.
func CompileFile(ctx context.Context, path string) (*CompiledSpec, error) {
	compiled, err := app.CompileFile(ctx, path)
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
