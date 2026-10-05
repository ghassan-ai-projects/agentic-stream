package episodes

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

// Config configures episode assembly. Nil Execution selects assembly-only use.
// CostControl enables the optional durable aggregate cost budget.
type Config struct {
	Spec        *spec.CompiledSpec
	IDGenerator ids.Generator
	CostControl *costcontrol.Controller
	Execution   *ExecutionConfig
}

// ExecutionConfig requires a database, executor and decision-epoch check.
// Shadow persistence is required when the configured spec dispatches in shadow mode.
type ExecutionConfig struct {
	DB            *storage.DB
	Executor      Executor
	Clock         clock.Clock
	OwnerEpoch    string
	DecisionEpoch func(context.Context, *sql.Tx, string) error
	ShadowStore   *qualification.ShadowStore
	Telemetry     *telemetry.Runtime
}

// New validates configuration and constructs the public episode service.
func New(cfg Config) (*Service, error) {
	application, err := app.New(applicationConfig(cfg))
	if err != nil {
		return nil, err
	}
	return &Service{app: application}, nil
}

func applicationConfig(cfg Config) app.Config {
	return app.Config{Spec: cfg.Spec, IDGenerator: cfg.IDGenerator, CostControl: cfg.CostControl, Execution: executionConfig(cfg.Execution)}
}

func executionConfig(cfg *ExecutionConfig) *app.ExecutionConfig {
	if cfg == nil {
		return nil
	}
	return &app.ExecutionConfig{Episodes: store.New(cfg.DB), Executor: cfg.Executor, Clock: cfg.Clock, OwnerEpoch: cfg.OwnerEpoch, DecisionEpoch: cfg.DecisionEpoch, ShadowStore: cfg.ShadowStore, Telemetry: cfg.Telemetry}
}
