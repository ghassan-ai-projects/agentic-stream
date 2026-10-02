package runtime

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func (p *Pipeline) executeAdmittedEpisodes(ctx context.Context, report *PipelineReport) error {
	for {
		processed, runErr := p.runner.RunOnce(ctx, p.tenantID)
		if runErr != nil {
			return fmt.Errorf("run episode: %w", runErr)
		}
		if !processed {
			break
		}
		report.EpisodesExecuted++
	}
	return nil
}

func (p *Pipeline) dispatchApprovedCommands(ctx context.Context, report *PipelineReport) error {
	for {
		dispatched, dispatchErr := p.dispatcher.DispatchOnce(ctx)
		if dispatchErr != nil {
			return fmt.Errorf("dispatch action: %w", dispatchErr)
		}
		if !dispatched {
			break
		}
		report.CommandsDispatched++
	}
	return nil
}

func (p *Pipeline) observeBatch(report PipelineReport) {
	if p.telemetry != nil {
		p.telemetry.ObservePipeline(telemetry.PipelineReport{
			EventsIngested: report.EventsIngested, EventsProcessed: report.EventsProcessed,
			EpisodesAdmitted: report.EpisodesAdmitted, EpisodesExecuted: report.EpisodesExecuted,
			IntentsEvaluated: report.IntentsEvaluated, CommandsDispatched: report.CommandsDispatched,
		})
	}
}
