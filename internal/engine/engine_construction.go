package engine

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// NewEngine creates an engine for the given spec and tenant.
func NewEngine(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string) (*Engine, error) {
	return newEngine(ctx, db, log, clk, compiled, tenantID, true)
}

// NewStreamEngine creates an engine that processes ingress, operators, and
// Situation versions without evaluating cognition. Replay uses this path for
// its deterministic stream projection.
func NewStreamEngine(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string) (*Engine, error) {
	return newEngine(ctx, db, log, clk, compiled, tenantID, false)
}

func newEngine(ctx context.Context, db *storage.DB, log *eventlog.EventLog, clk clock.Clock, compiled *spec.CompiledSpec, tenantID string, cognitionEnabled bool) (*Engine, error) {
	if tenantID == "" {
		tenantID = contractsv1.TenantID
	}
	if err := spec.SaveDeployment(ctx, db, tenantID, compiled); err != nil {
		return nil, fmt.Errorf("save deployment: %w", err)
	}
	configureSchemaValidation(log, compiled)
	engine := &Engine{db: db, log: log, clock: clk, spec: compiled, tenantID: tenantID, deploymentID: compiled.Digest}
	if err := engine.buildPlanes(ctx, cognitionEnabled); err != nil {
		return nil, err
	}
	return engine, nil
}

// buildPlanes creates the operator runtime and situation engine over one
// deterministic ID sequence, restores durable Situations, then attaches
// cognition.
func (e *Engine) buildPlanes(ctx context.Context, cognitionEnabled bool) error {
	idGen := ids.Deterministic()
	var err error
	if e.opRuntime, err = operators.NewOperatorRuntime(e.spec.Digest, e.spec, idGen); err != nil {
		return fmt.Errorf("operator runtime: %w", err)
	}
	if e.sitEngine, err = situations.NewEngine(e.spec.Digest, e.tenantID, 0, e.spec, idGen); err != nil {
		return fmt.Errorf("situation engine: %w", err)
	}
	if err := restoreSituations(ctx, e.db, e.spec.Digest, e.tenantID, e.sitEngine); err != nil {
		return fmt.Errorf("restore situations: %w", err)
	}
	e.cogEngine, err = newCognitionEngine(e.spec, e.tenantID, e.clock, idGen, cognitionEnabled)
	return err
}

func configureSchemaValidation(log *eventlog.EventLog, compiled *spec.CompiledSpec) {
	if len(compiled.Inputs) == 0 {
		return
	}
	for _, input := range compiled.Inputs {
		if input.SchemaRef == "" {
			return
		}
	}
	log.RequireSchemaValidation()
}

func newCognitionEngine(compiled *spec.CompiledSpec, tenantID string, clk clock.Clock, idGen ids.Generator, enabled bool) (*cognition.Service, error) {
	if !enabled {
		return nil, nil
	}
	engine, err := cognition.New(cognition.Config{DeploymentID: compiled.Digest, TenantID: tenantID, Spec: compiled, IDGen: idGen, Clock: clk})
	if err != nil {
		return nil, fmt.Errorf("cognition engine: %w", err)
	}
	return engine, nil
}
