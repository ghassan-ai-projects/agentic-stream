package app

import (
	"context"
	"fmt"
	"time"
)

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
			if _, err := p.Advance(ctx); err != nil && ctx.Err() == nil {
				return err
			}
		}
	}
}

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

func (p *Pipeline) lockBatch() (func(), error) {
	if p == nil {
		return nil, fmt.Errorf("pipeline is nil")
	}
	p.batchMu.Lock()
	return p.batchMu.Unlock, nil
}
