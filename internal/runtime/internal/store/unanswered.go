package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func (p *PipelineStore) UnansweredReasoning(ctx context.Context) ([]episodeledger.Unanswered, error) {
	unanswered, err := episodeledger.UnansweredReasoning(ctx, p.DB.DB, p.TenantID)
	if err != nil {
		return nil, fmt.Errorf("read unanswered reasoning: %w", err)
	}
	return unanswered, nil
}
