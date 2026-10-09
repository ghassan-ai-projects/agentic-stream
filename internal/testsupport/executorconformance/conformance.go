// Package executorconformance is test support: the semantic contract of the
// episodes.Executor port, run by every concrete executor (fixture, native and
// remote), plus the request builders they share. Import it from tests only.
package executorconformance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

func ticketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false,
		"properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}

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
		RequestJSON: fixtureRequestJSON(promptDigest, objectiveDigest),
	}
}

// fixtureRequestJSON is the durable request payload with a compiled one-intent
// catalog bound to the prompt and objective digests.
func fixtureRequestJSON(promptDigest, objectiveDigest string) []byte {
	intentCatalog, intentDigest, err := episodes.CompileIntentCatalog([]spec.Intent{
		{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		panic(err)
	}
	intentCatalogJSON, _ := json.Marshal(intentCatalog)
	return []byte(fmt.Sprintf(`{"kind":"standard","snapshot":{"phase":"warning"},"trigger":{"trigger_id":"trg-1","trigger_name":"warning","lane":"deep"},"tools":[],"allowed_intent_types":["create_maintenance_ticket"],"risk_ceiling":"R1","executor":{"objective":"diagnose","prompt_sha256":%q,"objective_sha256":%q,"decision_schema":{},"intent_catalog":%s,"intent_catalog_sha256":%q},"budget":{"wall_time":"5s"}}`, promptDigest, objectiveDigest, string(intentCatalogJSON), intentDigest))
}

// Run executes the shared semantic checks against one executor: a standard
// request yields a produced Outcome bound to the request's fenced attempt,
// carrying a Decision bound to the same episode, attempt, fence and snapshot
// with a verifiable digest.
func Run(ctx context.Context, executor episodes.Executor) error {
	if executor == nil {
		return errors.New("executor is required")
	}
	req := FixtureRequest()
	outcome, err := executor.Execute(ctx, req)
	if err != nil {
		return fmt.Errorf("execute conformance fixture: %w", err)
	}
	return checkProducedOutcome(req, outcome)
}

// RunCanceled requires an executor started with an already canceled context
// to end as the episode runner records a cancellation: either by returning an
// error that matches context.Canceled, or by returning the canceled Outcome
// for the request's attempt. It must not produce a Decision.
func RunCanceled(ctx context.Context, executor episodes.Executor) error {
	if executor == nil {
		return errors.New("executor is required")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	req := FixtureRequest()
	outcome, err := executor.Execute(canceled, req)
	return checkCanceledResult(req, outcome, err)
}

// checkCanceledResult accepts a context.Canceled error or the canceled
// Outcome of the request's attempt, and nothing else.
func checkCanceledResult(req *episodes.Request, outcome *episodes.Outcome, err error) error {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("canceled execution returned %w, want a context.Canceled ending", err)
	}
	if outcome == nil {
		return errors.New("canceled execution returned neither an outcome nor an error")
	}
	return CheckContextEnding(req, outcome, context.Canceled)
}

// SetBudget replaces the budget document of the request's durable payload.
func SetBudget(req *episodes.Request, budget map[string]any) {
	EditPayload(req, func(payload map[string]any) { payload["budget"] = budget })
}

// EditPayload decodes the request's durable payload, applies edit and encodes
// it back. The fixture payload is well formed, so a failure is a defect of the
// caller's edit and panics.
func EditPayload(req *episodes.Request, edit func(payload map[string]any)) {
	var payload map[string]any
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		panic(err)
	}
	edit(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	req.RequestJSON = encoded
}

// checkProducedOutcome requires a produced outcome bound to the request's
// attempt identity with a verifiable Decision digest and a Decision bound to
// the request.
func checkProducedOutcome(req *episodes.Request, outcome *episodes.Outcome) error {
	if outcome == nil || outcome.Status != string(episodeledger.AttemptProduced) {
		return fmt.Errorf("expected produced outcome, got %#v", outcome)
	}
	if outcome.AttemptID != req.AttemptID || outcome.Fence != req.Fence {
		return fmt.Errorf("outcome identity mismatch: %#v", outcome)
	}
	var decision map[string]any
	if err := json.Unmarshal(outcome.DecisionJSON, &decision); err != nil {
		return fmt.Errorf("outcome decision is not a JSON object: %w", err)
	}
	if !canonicaljson.Verify(canonicaljson.DomainDecision, decision, outcome.DecisionSHA256) {
		return errors.New("outcome decision digest is invalid")
	}
	return checkDecisionBinding(req, decision)
}

func checkDecisionBinding(req *episodes.Request, decision map[string]any) error {
	want := map[string]any{
		"episode_id": req.EpisodeID, "attempt_id": req.AttemptID, "fence": float64(req.Fence),
		"situation_id": req.SituationID, "situation_version": float64(req.SituationVersion), "snapshot_digest": req.SnapshotSHA256,
	}
	for field, value := range want {
		if decision[field] != value {
			return fmt.Errorf("decision %s = %v, want %v", field, decision[field], value)
		}
	}
	return nil
}

// CheckContextEnding requires outcome to be what the episode runner records
// when an executor returns ending (context.Canceled or
// context.DeadlineExceeded) as its error: the same attempt status and reason,
// so an executor that reports the ending in its Outcome and one that returns
// the error leave the same durable attempt.
func CheckContextEnding(req *episodes.Request, outcome *episodes.Outcome, ending error) error {
	want := req.ContextEndingOutcome(ending, outcome.CostMicrounits)
	if outcome.Status != want.Status || !slices.Equal(outcome.Reasons, want.Reasons) || outcome.AttemptID != req.AttemptID || outcome.Fence != req.Fence {
		return fmt.Errorf("outcome for %q = %+v, want status %q reasons %q bound to attempt %s fence %d", ending.Error(), outcome, want.Status, want.Reasons, req.AttemptID, req.Fence)
	}
	return nil
}
