package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
)

type AuditQueue = func(ctx context.Context, tx *sql.Tx, request domain.AuditRequest) (string, error)

type EpisodeDecision struct {
	Ended    bool
	Accepted bool
	Decision map[string]any
}

func (t *Tx) EpisodeEnded(ctx context.Context, episodeID string) (bool, error) {
	fence, found, err := episodeledger.ReadEpisodeFence(ctx, t.tx, episodeID)
	if err != nil {
		return false, fmt.Errorf("read the lifecycle of episode %s: %w", episodeID, err)
	}
	return found && !fence.Lifecycle.Live(), nil
}

func (t *Tx) Decision(ctx context.Context, episodeID string) (EpisodeDecision, error) {
	ended, err := t.EpisodeEnded(ctx, episodeID)
	if err != nil || !ended {
		return EpisodeDecision{}, err
	}
	views, err := episodes.Decisions(ctx, t.db, episodeID)
	if err != nil {
		return EpisodeDecision{}, fmt.Errorf("read the decisions of episode %s: %w", episodeID, err)
	}
	return lastDecision(views)
}

func lastDecision(views []episodes.DecisionView) (EpisodeDecision, error) {
	if len(views) == 0 {
		return EpisodeDecision{Ended: true}, nil
	}
	last := views[len(views)-1]
	var decision map[string]any
	if err := json.Unmarshal(last.Decision, &decision); err != nil {
		return EpisodeDecision{}, fmt.Errorf("decode decision %s: %w", last.DecisionID, err)
	}
	return EpisodeDecision{Ended: true, Accepted: last.Accepted(), Decision: decision}, nil
}

func (t *Tx) QueueAudit(ctx context.Context, queue AuditQueue, request domain.AuditRequest) (string, error) {
	itemID, err := queue(ctx, t.tx, request)
	if err != nil {
		return "", fmt.Errorf("queue the audit of situation %s v%d: %w", request.SituationID, request.SituationVersion, err)
	}
	return itemID, nil
}
