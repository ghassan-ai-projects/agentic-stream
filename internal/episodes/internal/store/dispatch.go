package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func DispatchableEpisode(ctx context.Context, tx *Tx, tenantID string, killedSuperseded bool) (episodeledger.DispatchableEpisode, bool, error) {
	episode, found, err := episodeledger.NextDispatchableEpisode(ctx, tx.tx, tenantID, killedSuperseded)
	if err != nil {
		return episodeledger.DispatchableEpisode{}, false, fmt.Errorf("read dispatchable episode: %w", err)
	}
	return episode, found, nil
}
