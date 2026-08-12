// Package conformance contains the semantic executor contract shared by the
// in-process Go fixture and the streamed EpisodeWorker fixture.
package conformance

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
)

// FixtureRequest is the smallest valid request shape used by conformance.
func FixtureRequest() *episodes.Request {
	promptDigest, _ := canonicaljson.Digest(canonicaljson.DomainPrompt, map[string]any{"version": "prompt-v1"})
	objectiveDigest, _ := canonicaljson.Digest(canonicaljson.DomainObjective, map[string]any{"text": "diagnose"})
	return &episodes.Request{
		EpisodeID: "epi-conformance", TenantID: "tenant", SituationID: "sit-conformance", SituationVersion: 1,
		ExecutorName: "native", ExecutorVersion: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotSHA256: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		AttemptID:      "att-conformance", Fence: 1,
		PromptSHA256: promptDigest, ObjectiveSHA256: objectiveDigest,
		RequestJSON: []byte(fmt.Sprintf(`{"kind":"diagnose","snapshot":{"phase":"warning"},"trigger":{"trigger_id":"trg-1","trigger_name":"warning","lane":"deep"},"tools":[],"allowed_intent_types":["create_maintenance_ticket"],"risk_ceiling":"R1","executor":{"objective":"diagnose","prompt_sha256":%q,"objective_sha256":%q,"decision_schema":{}},"budget":{"wall_time":"5s"}}`, promptDigest, objectiveDigest)),
	}
}

// Run executes the shared semantic checks against one executor.
func Run(ctx context.Context, executor episodes.Executor) error {
	if executor == nil {
		return fmt.Errorf("executor is required")
	}
	req := FixtureRequest()
	outcome, err := executor.Execute(ctx, req)
	if err != nil {
		return fmt.Errorf("execute conformance fixture: %w", err)
	}
	if outcome == nil || outcome.Status != string(episodes.AttemptProduced) {
		return fmt.Errorf("expected produced outcome, got %#v", outcome)
	}
	if outcome.AttemptID != req.AttemptID || outcome.Fence != req.Fence {
		return fmt.Errorf("outcome identity mismatch: %#v", outcome)
	}
	if len(outcome.DecisionJSON) == 0 || !canonicaljson.Verify(canonicaljson.DomainDecision, mustDocument(outcome.DecisionJSON), outcome.DecisionSHA256) {
		return fmt.Errorf("outcome decision digest is invalid")
	}
	return nil
}

func mustDocument(raw []byte) map[string]any {
	var document map[string]any
	_ = json.Unmarshal(raw, &document)
	return document
}
