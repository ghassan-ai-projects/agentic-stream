package episodes

import (
	"context"
	"encoding/json"
	"fmt"
)

// FakeExecutor is a deterministic executor for tests and replay. It produces a
// structured Decision without calling a real model.
type FakeExecutor struct{}

// NewFakeExecutor creates a deterministic fake executor.
func NewFakeExecutor() *FakeExecutor {
	return &FakeExecutor{}
}

// Name returns the executor identifier.
func (e *FakeExecutor) Name() string { return "fake" }

// Execute returns a deterministic Decision based on the request snapshot.
func (e *FakeExecutor) Execute(ctx context.Context, req *Request) (*Outcome, error) {
	_ = ctx

	var payload map[string]any
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	snapshot, _ := payload["snapshot"].(map[string]any)
	trigger, _ := payload["trigger"].(map[string]any)

	phase := "unknown"
	if p, ok := snapshot["phase"].(string); ok {
		phase = p
	}
	triggerName := "unknown"
	if t, ok := trigger["trigger_name"].(string); ok {
		triggerName = t
	}

	decision := map[string]any{
		"episode_id":        req.EpisodeID,
		"situation_id":      req.SituationID,
		"situation_version": req.SituationVersion,
		"summary":           fmt.Sprintf("fake decision for phase %s via trigger %s", phase, triggerName),
		"confidence":        0.95,
		"facts_used": []map[string]any{
			{"path": "snapshot.phase", "value": phase},
		},
		"intents": []map[string]any{
			{"type": "create_maintenance_ticket", "risk": "R1", "parameters": map[string]any{"reason": phase}},
		},
	}
	decisionJSON, err := json.Marshal(decision)
	if err != nil {
		return nil, fmt.Errorf("marshal decision: %w", err)
	}

	return &Outcome{
		Status:       string(AttemptProduced),
		DecisionJSON: decisionJSON,
		Reasons:      []string{"deterministic fake outcome"},
	}, nil
}
