package app

import (
	"context"
	"database/sql"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func (r *Runner) WithDecisionEpoch(check store.OwnerCheck) *Runner {
	r.decisionEpoch = check
	return r
}

func (r *Runner) WithAssembler(assembler *Assembler) *Runner {
	r.assembler = assembler
	return r
}

func (r *Runner) WithCostLedger(ledger *runtimecontrol.CostLedger) *Runner {
	r.cost = ledger
	return r
}

func (r *Runner) WithTelemetry(runtime *telemetry.Runtime) *Runner {
	r.telemetry = runtime
	return r
}

func (a *Assembler) WithCostLedger(ledger *runtimecontrol.CostLedger) *Assembler {
	a.cost = ledger
	return a
}

func NewAssembler(compiled *spec.CompiledSpec, idGen sources.Generator) *Assembler {
	return &Assembler{spec: compiled, idGen: sources.OrRandom(idGen)}
}

func NewRunner(db store.Store, executor Executor, clk sources.Clock, idGen sources.Generator) *Runner {
	return &Runner{episodes: db, executor: executor, clk: sources.OrPhysical(clk), idGen: sources.OrRandom(idGen), decisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }}
}
