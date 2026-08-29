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

	idGen := ids.Deterministic()
	opRuntime, err := operators.NewOperatorRuntime(compiled.Digest, compiled, idGen)
	if err != nil {
		return nil, fmt.Errorf("operator runtime: %w", err)
	}
	sitEngine, err := situations.NewEngine(compiled.Digest, tenantID, 0, compiled, idGen)
	if err != nil {
		return nil, fmt.Errorf("situation engine: %w", err)
	}
	if err := restoreSituations(ctx, db, compiled.Digest, tenantID, sitEngine); err != nil {
		return nil, fmt.Errorf("restore situations: %w", err)
	}
	cogEngine, err := newCognitionEngine(db, compiled, tenantID, clk, idGen, cognitionEnabled)
	if err != nil {
		return nil, err
	}
	return &Engine{
		db: db, log: log, clock: clk, spec: compiled,
		tenantID: tenantID, deploymentID: compiled.Digest,
		opRuntime: opRuntime, sitEngine: sitEngine, cogEngine: cogEngine,
	}, nil
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

func newCognitionEngine(db *storage.DB, compiled *spec.CompiledSpec, tenantID string, clk clock.Clock, idGen ids.Generator, enabled bool) (*cognition.Engine, error) {
	if !enabled {
		return nil, nil
	}
	engine, err := cognition.NewEngine(db, compiled.Digest, tenantID, compiled, idGen, clk)
	if err != nil {
		return nil, fmt.Errorf("cognition engine: %w", err)
	}
	return engine, nil
}
