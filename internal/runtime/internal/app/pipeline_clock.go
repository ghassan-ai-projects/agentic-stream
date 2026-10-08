package app

import (
	"context"
	"fmt"
	"time"
)

// AdvanceEvery advances the pipeline on a schedule until ctx ends, so that
// time-driven work makes progress while the source is quiet: due timers
// (windows, missing heartbeats), debounced and cooled-down cognition, intents
// waiting for policy and commands approved by a human. A live source that only
// advanced on new evidence would never notice its own silence.
func (p *Pipeline) AdvanceEvery(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("pipeline advance interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := p.Advance(ctx); err != nil {
				return err
			}
		}
	}
}

// Advance runs every stage after ingestion without new evidence. It is
// serialized with ingestion, so deterministic state still changes one batch at
// a time.
func (p *Pipeline) Advance(ctx context.Context) (PipelineReport, error) {
	unlock, err := p.lockBatch()
	if err != nil {
		return PipelineReport{}, err
	}
	defer unlock()
	before, err := p.prepareIngest(ctx)
	if err != nil {
		return PipelineReport{}, err
	}
	return p.runAfterIngest(ctx, PipelineReport{}, before)
}

// lockBatch takes the batch lock that keeps source batches and scheduled
// advances serial, and returns its release.
func (p *Pipeline) lockBatch() (func(), error) {
	if p == nil {
		return nil, fmt.Errorf("pipeline is nil")
	}
	p.batchMu.Lock()
	return p.batchMu.Unlock, nil
}
