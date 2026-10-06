package runtime

import (
	"context"

	app "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/app"
	composition "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/composition"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
)

// PipelineConfig configures one owner-scoped live pipeline.
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
