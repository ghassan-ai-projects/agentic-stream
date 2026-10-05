package app

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/admission"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

// PipelineDependencies are fully composed planes; app owns no database handle.
type PipelineDependencies struct {
	Log          *eventlog.EventLog
	Engine       *engine.Engine
	Admission    *admission.Admitter
	Runner       *episodes.Runner
	Dispatcher   *actions.Dispatcher
	Watch        *watch.Effector
	Telemetry    *telemetry.Runtime
	Transactions *store.PipelineStore
	Sources      *transport.Sources
	Clock        clock.Clock
	TenantID     string
}

// NewPipeline joins the supplied planes into one owner-scoped orchestrator.
func NewPipeline(d PipelineDependencies) *Pipeline {
	return &Pipeline{log: d.Log, engine: d.Engine, admission: d.Admission, runner: d.Runner, dispatcher: d.Dispatcher, watch: d.Watch, telemetry: d.Telemetry, transactions: d.Transactions, sources: d.Sources, clk: d.Clock, tenantID: d.TenantID}
}
