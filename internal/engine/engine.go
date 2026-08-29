package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ConsumerName identifies the engine's inbox consumer.
const ConsumerName = "engine"

// Engine processes events for a single virtual partition deterministically.
type Engine struct {
	mu           sync.Mutex
	db           *storage.DB
	log          *eventlog.EventLog
	clock        clock.Clock
	spec         *spec.CompiledSpec
	tenantID     string
	deploymentID string
	owner        *storage.RuntimeOwner
	ownerEpoch   string

	opRuntime *operators.OperatorRuntime
	sitEngine *situations.Engine
	cogEngine *cognition.Engine
}

// WithRuntimeOwner fences stream state transactions to the active runtime
// lease. It is used by live composition; deterministic replay leaves it unset.
func (e *Engine) WithRuntimeOwner(owner *storage.RuntimeOwner, epoch string) *Engine {
	e.owner = owner
	e.ownerEpoch = epoch
	return e
}

// Run reads and applies events for partitionID until no more unprocessed
// records remain. It returns the number of events processed.
func (e *Engine) Run(ctx context.Context, partitionID int) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.run(ctx, partitionID, nil)
}

// RunWithHook processes a partition and calls beforeApply immediately before
// each event is applied. The hook runs outside the engine transaction and is
// intended for deterministic replay clocks only.
func (e *Engine) RunWithHook(ctx context.Context, partitionID int, beforeApply func(eventlog.Record) error) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.run(ctx, partitionID, beforeApply)
}

// RunGlobal applies all partitions in durable event-log position order. It is
// used by replay so one virtual clock cannot observe a later partition before
// an earlier record in the authoritative trace.
func (e *Engine) RunGlobal(ctx context.Context, beforeApply func(eventlog.Record) error) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runGlobal(ctx, beforeApply)
}

// RunDueTimers applies durable processing-time timers whose due time has
// passed according to the runtime clock.
func (e *Engine) RunDueTimers(ctx context.Context, partitionID int) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runDueTimers(ctx, partitionID)
}

// RunTimerLoop drives durable processing-time timers for a live runtime. The
// database remains authoritative; the clock timer is only a wake-up mechanism.
func (e *Engine) RunTimerLoop(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	for {
		e.mu.Lock()
		_, err := e.runDueTimersForAllPartitions(ctx)
		e.mu.Unlock()
		if err != nil {
			return fmt.Errorf("drive timers: %w", err)
		}
		timer := e.clock.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			_ = timer.Stop()
			return fmt.Errorf("timer loop interrupted: %w", ctx.Err())
		case <-timer.C():
		}
	}
}
