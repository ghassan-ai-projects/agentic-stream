package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func (r *Request) EvidenceHorizon() (time.Time, error) {
	var request struct {
		Snapshot struct {
			EventHorizon string `json:"event_horizon"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(r.RequestJSON, &request); err != nil {
		return time.Time{}, fmt.Errorf("decode request snapshot horizon: %w", err)
	}
	horizon, err := kernel.ParseTime(request.Snapshot.EventHorizon)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse snapshot event horizon of episode %s: %w", r.EpisodeID, err)
	}
	return horizon, nil
}
