package fixture

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Executor is a deterministic executor for tests and replay. It produces a
// structured Decision without calling a real model.
type Executor struct{}

// New creates a deterministic fake executor.
func New() *Executor {
	return &Executor{}
}

// Execute returns a deterministic Decision based on the request snapshot.
func (e *Executor) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
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
	return fakeOutcome(req, decision)
}

// fakeExecutorInputs reads the snapshot phase and trigger name, defaulting
// each to "unknown".
func fakeExecutorInputs(req *episodes.Request) (string, string, error) {
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
func fakeIntent(req *episodes.Request, phase string) (map[string]any, error) {
	parameters := map[string]any{explanationParameter(req): phase}
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

func fakeMaintenanceIntent(req *episodes.Request, parameters map[string]any) map[string]any {
	return map[string]any{
		"intent_id":         sources.PrefixIntent + req.EpisodeID,
		"decision_id":       sources.PrefixDecision + req.EpisodeID,
		"tenant_id":         req.TenantID,
		"situation_id":      req.SituationID,
		"situation_version": req.SituationVersion,
		"type":              "create_maintenance_ticket",
		"risk_class":        "R1",
		"parameters":        parameters,
		"expires_at":        "2099-01-01T00:00:00.000000000Z",
	}
}

func fakeDecision(req *episodes.Request, phase, triggerName string, intent map[string]any) map[string]any {
	return map[string]any{
		"decision_id":       sources.PrefixDecision + req.EpisodeID,
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

// fakeOutcome seals the decision as a produced outcome and labels it as the
// fake executor's.
func fakeOutcome(req *episodes.Request, decision map[string]any) (*episodes.Outcome, error) {
	outcome, err := req.ProducedOutcome(decision, 0)
	if err != nil {
		return nil, fmt.Errorf("produce fake outcome: %w", err)
	}
	outcome.Reasons = []string{"deterministic fake outcome"}
	return outcome, nil
}

type requestCatalog struct {
	Executor struct {
		IntentCatalog []struct {
			Type            string `json:"type"`
			ParameterSchema struct {
				Properties map[string]any `json:"properties"`
			} `json:"parameter_schema"`
		} `json:"intent_catalog"`
	} `json:"executor"`
}

// explanationParameter names the ticket parameter that carries the phase: the
// catalog schema's "hypothesis" when it declares one, otherwise "reason".
func explanationParameter(req *episodes.Request) string {
	var payload requestCatalog
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return "reason"
	}
	for _, entry := range payload.Executor.IntentCatalog {
		if _, declared := entry.ParameterSchema.Properties["hypothesis"]; entry.Type == "create_maintenance_ticket" && declared {
			return "hypothesis"
		}
	}
	return "reason"
}
