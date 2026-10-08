package app

import (
	"context"
	"fmt"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Config binds request assembly and optional execution dependencies.
type Config struct {
	Spec        *spec.CompiledSpec
	IDGenerator sources.Generator
	CostControl *runtimecontrol.CostLedger
	Execution   *ExecutionConfig
}

// ExecutionConfig binds required execution ports to one runner.
type ExecutionConfig struct {
	Episodes      store.Store
	Executor      Executor
	Clock         sources.Clock
	OwnerEpoch    string
	DecisionEpoch store.DecisionEpochCheck
	Telemetry     *telemetry.Runtime
}

// Service orders the episode use cases behind the public facade.
type Service struct {
	assembler *Assembler
	runner    *Runner
}

// New rejects missing dependencies before an episode can dispatch.
func New(cfg Config) (*Service, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	cfg.IDGenerator = sources.OrRandom(cfg.IDGenerator)
	assembler := &Assembler{spec: cfg.Spec, idGen: cfg.IDGenerator, cost: cfg.CostControl}
	return &Service{assembler: assembler, runner: configuredRunner(cfg, assembler)}, nil
}
func validateConfig(cfg Config) error {
	if cfg.Spec == nil {
		return fmt.Errorf("episode spec is required")
	}
	if cfg.Execution == nil {
		return nil
	}
	return validateExecution(cfg)
}
func validateExecution(cfg Config) error {
	execution := cfg.Execution
	if !execution.Episodes.Configured() || execution.Executor == nil || execution.DecisionEpoch == nil {
		return fmt.Errorf("episode execution database, executor and decision epoch check are required")
	}
	return nil
}
func configuredRunner(cfg Config, assembler *Assembler) *Runner {
	if cfg.Execution == nil {
		return nil
	}
	execution := *cfg.Execution
	execution.Clock = sources.OrPhysical(execution.Clock)
	return &Runner{episodes: execution.Episodes, executor: execution.Executor, clk: execution.Clock, idGen: cfg.IDGenerator, ownerEpoch: execution.OwnerEpoch, cost: cfg.CostControl, decisionEpoch: execution.DecisionEpoch, telemetry: execution.Telemetry, assembler: assembler}
}

// Assemble builds a situation-bound request in the joined transaction.
func (s *Service) Assemble(ctx context.Context, tx *store.Tx, item, tenant string) (*Request, error) {
	return s.assembler.Assemble(ctx, tx, item, tenant)
}

// Persist admits a request in the joined transaction.
func (s *Service) Persist(ctx context.Context, tx *store.Tx, req *Request, now time.Time) error {
	return s.assembler.Persist(ctx, tx, req, now)
}

// RunOnce refuses assembly-only configuration before accessing execution state.
func (s *Service) RunOnce(ctx context.Context, tenant string) (bool, error) {
	if s.runner == nil {
		return false, fmt.Errorf("episode service is configured for assembly only")
	}
	return s.runner.RunOnce(ctx, tenant)
}
