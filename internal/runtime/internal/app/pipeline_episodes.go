package app

import (
	"context"
	"fmt"
	"time"
)

func (p *Pipeline) RunEpisodesEvery(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("pipeline episode interval must be positive")
	}
	p.episodesBeside.Store(true)
	defer p.episodesBeside.Store(false)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	return p.executeEpisodesOnSchedule(ctx, ticker.C)
}

func (p *Pipeline) executeEpisodesOnSchedule(ctx context.Context, ticks <-chan time.Time) error {
	for {
		if err := p.executeDueEpisodes(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticks:
		}
	}
}

func (p *Pipeline) executeDueEpisodes(ctx context.Context) error {
	var report PipelineReport
	if err := p.executeAdmittedEpisodes(ctx, &report); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
