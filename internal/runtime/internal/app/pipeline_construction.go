package app

import (
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

type PipelineDependencies struct {
	Log          *eventlog.EventLog
	Engine       *engine.Service
	Admission    *Admitter
	Runner       *episodes.Service
	Dispatcher   *actions.Service
	Watch        *watch.Service
	Telemetry    *telemetry.Runtime
	Transactions *store.PipelineStore
	Sources      *transport.Sources
	Clock        sources.Clock
	TenantID     string

	MaintenanceInterval time.Duration
}

func NewPipeline(d PipelineDependencies) *Pipeline {
	return &Pipeline{log: d.Log, engine: d.Engine, admission: d.Admission, runner: d.Runner, dispatcher: d.Dispatcher, watch: d.Watch, situationFeatures: d.Engine.SituationFeatures, telemetry: d.Telemetry, transactions: d.Transactions, sources: d.Sources, clk: d.Clock, tenantID: d.TenantID, maintenanceInterval: d.MaintenanceInterval}
}
