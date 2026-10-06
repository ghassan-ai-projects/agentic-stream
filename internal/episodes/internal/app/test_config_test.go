package app

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// WithDecisionEpoch installs an explicit test fence without reimplementing refusal rules.
func (r *Runner) WithDecisionEpoch(check store.DecisionEpochCheck) *Runner {
	r.decisionEpoch = check
	return r
}

// NewAssembler creates an assembler for the given spec.
func NewAssembler(compiled *spec.CompiledSpec, idGen sources.Generator) *Assembler {
	if idGen == nil {
		idGen = sources.Random()
	}
	return &Assembler{spec: compiled, idGen: idGen}
}

// WithAssembler enables the ISSUE-061 re-bind path: an admitted episode whose
// situation advanced past its bound version is re-bound to the live version
// and dispatched instead of abandoned. Without an assembler the runner keeps
// the pre-fix abandon behavior (tests and minimal wiring).
func (r *Runner) WithAssembler(assembler *Assembler) *Runner {
	r.assembler = assembler
	return r
}

// NewRunner creates an isolated test runner with an explicit permissive fixture fence.
func NewRunner(db store.Store, executor Executor, clk sources.Clock, idGen sources.Generator) *Runner {
	if clk == nil {
		clk = sources.Physical()
	}
	if idGen == nil {
		idGen = sources.Random()
	}
	return &Runner{episodes: db, executor: executor, clk: clk, idGen: idGen, decisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }}
}
