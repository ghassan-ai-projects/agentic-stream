package episodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// FakeExecutor is a deterministic executor for tests and replay. It produces a
// structured Decision without calling a real model.
type FakeExecutor struct{}

// NewFakeExecutor creates a deterministic fake executor.
func NewFakeExecutor() *FakeExecutor {
	return &FakeExecutor{}
}

// Execute returns a deterministic Decision based on the request snapshot.
func (e *FakeExecutor) Execute(ctx context.Context, req *Request) (*Outcome, error) {
	_ = ctx
	phase, triggerName, err := fakeExecutorInputs(req)
	if err != nil {
		return nil, err
	}
	intent, err := fakeIntent(req, phase)
	if err != nil {
		return nil, err
	}
	decision := fakeDecision(req, phase, triggerName, intent)
	return producedOutcome(req, decision)
}

// fakeExecutorInputs reads the snapshot phase and trigger name, defaulting
// each to "unknown".
func fakeExecutorInputs(req *Request) (string, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return "", "", fmt.Errorf("unmarshal request: %w", err)
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
	return phase, triggerName, nil
}

// fakeIntent is a digest-bound R1 maintenance-ticket intent for the episode.
func fakeIntent(req *Request, phase string) (map[string]any, error) {
	parameters := map[string]any{"reason": phase}
	if req.EntityID != "" {
		parameters["entity_id"] = req.EntityID
	}
	intent := fakeMaintenanceIntent(req, parameters)
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		return nil, fmt.Errorf("digest intent: %w", err)
	}
	intent["intent_digest"] = intentDigest
	return intent, nil
}

func fakeMaintenanceIntent(req *Request, parameters map[string]any) map[string]any {
	return map[string]any{
		"intent_id":         "int_" + req.EpisodeID,
		"decision_id":       "dec_" + req.EpisodeID,
		"tenant_id":         req.TenantID,
		"situation_id":      req.SituationID,
		"situation_version": req.SituationVersion,
		"type":              "create_maintenance_ticket",
		"risk_class":        "R1",
		"parameters":        parameters,
		"expires_at":        "2099-01-01T00:00:00.000000000Z",
	}
}

func fakeDecision(req *Request, phase, triggerName string, intent map[string]any) map[string]any {
	return map[string]any{
		"decision_id":       "dec_" + req.EpisodeID,
		"episode_id":        req.EpisodeID,
		"attempt_id":        req.AttemptID,
		"fence":             req.Fence,
		"snapshot_digest":   req.SnapshotSHA256,
		"situation_id":      req.SituationID,
		"situation_version": req.SituationVersion,
		"summary":           fmt.Sprintf("fake decision for phase %s via trigger %s", phase, triggerName),
		"confidence":        0.95,
		"facts_used":        []map[string]any{{"path": "snapshot.phase", "value": phase}},
		"intents":           []map[string]any{intent},
	}
}

// producedOutcome canonicalizes and digests the decision as a produced
// outcome for the request's attempt.
func producedOutcome(req *Request, decision map[string]any) (*Outcome, error) {
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return nil, fmt.Errorf("marshal decision: %w", err)
	}
	decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return nil, fmt.Errorf("digest decision: %w", err)
	}
	return &Outcome{
		Status: string(episodeledger.AttemptProduced), AttemptID: req.AttemptID, Fence: req.Fence,
		DecisionJSON:   decisionJSON,
		DecisionSHA256: decisionDigest,
		Reasons:        []string{"deterministic fake outcome"},
	}, nil
}
