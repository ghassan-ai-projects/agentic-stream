package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// DispatchableEpisode reads the oldest admitted or running episode inside the
// caller's transaction; found is false when nothing is dispatchable. With
// killedSuperseded set it also admits superseded episodes without an attempt
// under a killed policy epoch, so the runner can release their reservation and
// quarantine them.
func DispatchableEpisode(ctx context.Context, tx *Tx, tenantID string, killedSuperseded bool) (episodeledger.DispatchableEpisode, bool, error) {
	episode, found, err := episodeledger.NextDispatchableEpisode(ctx, tx.tx, tenantID, killedSuperseded)
	if err != nil {
		return episodeledger.DispatchableEpisode{}, false, fmt.Errorf("read dispatchable episode: %w", err)
	}
	return episode, found, nil
}
