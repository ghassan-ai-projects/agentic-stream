package app

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	transport "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch"
)

type PipelineReport = domain.PipelineReport

const defaultWatchPageSize = 1000

type Pipeline struct {
	transactions *store.PipelineStore
	sources      *transport.Sources
	log          *eventlog.EventLog
	engine       *engine.Service
	admission    *Admitter
	runner       *episodes.Service
	dispatcher   *actions.Service
	watch        *watch.Service
	clk          sources.Clock
	tenantID     string

	maintenanceInterval time.Duration
	watchPageSize       int

	batchMu sync.Mutex

	episodesBeside atomic.Bool
	watchMu        sync.Mutex
	watchStop      context.CancelFunc
	watchDone      chan struct{}
	watchErr       error
	telemetry      *telemetry.Runtime
}

func (p *Pipeline) Start(ctx context.Context) error {
	if p == nil || p.watch == nil {
		return fmt.Errorf("pipeline watch maintenance is not configured")
	}
	p.watchMu.Lock()
	defer p.watchMu.Unlock()
	if p.watchStop != nil {
		return fmt.Errorf("pipeline is already started")
	}
	watchCtx, stop := context.WithCancel(ctx)
	p.watchStop = stop
	p.watchDone = make(chan struct{})
	done := p.watchDone
	go p.maintainWatches(watchCtx, done)
	return nil
}

func (p *Pipeline) maintainWatches(watchCtx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(domain.MaintenanceInterval(p.maintenanceInterval))
	defer ticker.Stop()
	for {
		select {
		case <-watchCtx.Done():
			return
		case <-ticker.C:
			if err := p.expireMaintainedWatches(watchCtx); err != nil {
				return
			}
		}
	}
}

func (p *Pipeline) expireMaintainedWatches(ctx context.Context) error {
	if err := p.maintainApprovedWork(ctx); err != nil {
		p.watchMu.Lock()
		p.watchErr = err
		p.watchMu.Unlock()
		return fmt.Errorf("%w", err)
	}
	return nil
}

func (p *Pipeline) Close() error {
	if p == nil {
		return nil
	}
	p.watchMu.Lock()
	stop, done := p.watchStop, p.watchDone
	p.watchStop = nil
	p.watchDone = nil
	p.watchMu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
	return nil
}

func (p *Pipeline) watchFailure() error {
	p.watchMu.Lock()
	defer p.watchMu.Unlock()
	return p.watchErr
}

func (p *Pipeline) assertOwner(ctx context.Context) error { return p.transactions.AssertOwner(ctx) }
