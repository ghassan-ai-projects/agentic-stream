package app

import (
	"context"
	"fmt"
	"time"
)

// RunEpisodesEvery executes admitted episodes on their own loop until ctx ends,
// checking for due work every interval (ADR-018). While it runs, ingestion,
// timers, cognition, policy and dispatch keep advancing during a worker's
// reasoning, cognition can supersede the running attempt, and the batch no
// longer executes episodes inline. One attempt runs at a time.
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

// executeEpisodesOnSchedule drains due episodes, then waits for the next tick.
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

// executeDueEpisodes runs every admitted episode that is due, one at a time,
// outside the batch lock. Cancellation ends the loop without an error.
func (p *Pipeline) executeDueEpisodes(ctx context.Context) error {
	var report PipelineReport
	if err := p.executeAdmittedEpisodes(ctx, &report); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
