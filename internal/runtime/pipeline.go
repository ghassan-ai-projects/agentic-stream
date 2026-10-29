package runtime

import (
	"context"
	"time"

	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	composition "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/composition"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
)

// PipelineConfig configures one owner-scoped live pipeline. Clock is the wall
// clock (default: physical) and times everything bounded by real time.
// DecisionClock times scheduling only (the debounce, cooldown and expiry of a
// trigger and the admitter's due and expired items) and defaults to Clock; a
// barrier-paced simulated world supplies one that follows the world's time.
type PipelineConfig = composition.PipelineConfig

// PipelineReport describes one completed live batch.
type PipelineReport = domain.PipelineReport

// Pipeline is the public facade over live orchestration.
type Pipeline struct{ application *app.Pipeline }

// NewPipeline composes the existing planes before constructing app use cases.
func NewPipeline(ctx context.Context, cfg PipelineConfig) (*Pipeline, error) {
	application, err := composition.NewPipeline(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Pipeline{application: application}, nil
}
func (p *Pipeline) useCases() *app.Pipeline {
	if p == nil {
		return nil
	}
	return p.application
}

// Start begins runtime-owned maintenance.
func (p *Pipeline) Start(ctx context.Context) error { return p.useCases().Start(ctx) }

// Close stops runtime-owned maintenance.
func (p *Pipeline) Close() error { return p.useCases().Close() }

// RunJSONL advances the normalized source through all governed stages.
func (p *Pipeline) RunJSONL(ctx context.Context, path string) (PipelineReport, error) {
	return p.useCases().RunJSONL(ctx, path)
}

// RunSimulatorJSONL advances a strict simulator source.
func (p *Pipeline) RunSimulatorJSONL(ctx context.Context, path string) (PipelineReport, error) {
	return p.useCases().RunSimulatorJSONL(ctx, path)
}

// RunLiveSocket owns a live source until cancellation or failure.
func (p *Pipeline) RunLiveSocket(ctx context.Context, path string) error {
	return p.useCases().RunLiveSocket(ctx, path)
}

// Feed is an in-process live source, such as a plant wire host: it runs until
// ctx ends and hands each normalized envelope it produces to emit.
type Feed = app.Feed

// RunFeed owns an in-process live source until cancellation or failure: each
// envelope it emits is admitted as a live line is and runs through the governed
// stages.
func (p *Pipeline) RunFeed(ctx context.Context, feed Feed) error {
	return p.useCases().RunFeed(ctx, feed)
}

// AdvanceEvery advances the pipeline on a schedule until ctx ends, so a quiet
// live source still makes time-driven progress.
func (p *Pipeline) AdvanceEvery(ctx context.Context, interval time.Duration) error {
	return p.useCases().AdvanceEvery(ctx, interval)
}

// RunEpisodesEvery executes admitted episodes beside ingestion until ctx ends,
// so a worker's reasoning never holds back evidence, timers or supersession.
func (p *Pipeline) RunEpisodesEvery(ctx context.Context, interval time.Duration) error {
	return p.useCases().RunEpisodesEvery(ctx, interval)
}

// HealthGauges reads the tenant's ingest lag, queue depths, set-aside records
// and database size; serve exposes them on /metrics at scrape time.
func (p *Pipeline) HealthGauges(ctx context.Context) (map[string]float64, error) {
	return p.useCases().HealthGauges(ctx) //nolint:wrapcheck // Facade operations only delegate; the use case names the failure.
}
