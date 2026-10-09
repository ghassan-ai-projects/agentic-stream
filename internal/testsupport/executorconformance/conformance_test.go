package executorconformance_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
)

const canceledStatus = string(episodeledger.AttemptCancelled)

type executorFunc func(context.Context, *episodes.Request) (*episodes.Outcome, error)

func (f executorFunc) Execute(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
	return f(ctx, req)
}

func producing(editDecision func(map[string]any), editOutcome func(*episodes.Outcome)) executorFunc {
	return func(_ context.Context, req *episodes.Request) (*episodes.Outcome, error) {
		decision := map[string]any{
			"decision_id": "dec", "episode_id": req.EpisodeID, "attempt_id": req.AttemptID, "fence": req.Fence,
			"situation_id": req.SituationID, "situation_version": req.SituationVersion, "snapshot_digest": req.SnapshotSHA256,
		}
		if editDecision != nil {
			editDecision(decision)
		}
		outcome, err := req.ProducedOutcome(decision, 0)
		if err != nil {
			return nil, err
		}
		if editOutcome != nil {
			editOutcome(outcome)
		}
		return outcome, nil
	}
}

type breach struct {
	name     string
	executor episodes.Executor
	want     string
}

func requireRefusals(t *testing.T, tests []breach, run func(context.Context, episodes.Executor) error) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := run(t.Context(), tt.executor); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("conformance run = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func tamperOutcome(change func(*episodes.Outcome)) executorFunc { return producing(nil, change) }

func tamperDecision(change func(map[string]any)) executorFunc { return producing(change, nil) }

func TestRunAcceptsAProducedOutcomeBoundToTheFencedAttempt(t *testing.T) {
	t.Parallel()
	if err := executorconformance.Run(t.Context(), producing(nil, nil)); err != nil {
		t.Fatalf("conforming executor rejected: %v", err)
	}
}

func TestRunRejectsEveryBreachOfTheOutcomeContract(t *testing.T) {
	t.Parallel()
	tests := []breach{
		{"no executor", nil, "executor is required"},
		{"execution error", executorFunc(func(context.Context, *episodes.Request) (*episodes.Outcome, error) {
			return nil, errors.New("boom")
		}), "execute conformance fixture: boom"},
		{"no outcome", executorFunc(func(context.Context, *episodes.Request) (*episodes.Outcome, error) { return nil, nil }), "expected produced outcome"},
		{"failed outcome", tamperOutcome(func(o *episodes.Outcome) { o.Status = "failed" }), "expected produced outcome"},
		{"foreign attempt", tamperOutcome(func(o *episodes.Outcome) { o.AttemptID = "other" }), "outcome identity mismatch"},
		{"stale fence", tamperOutcome(func(o *episodes.Outcome) { o.Fence++ }), "outcome identity mismatch"},
		{"decision is not json", tamperOutcome(func(o *episodes.Outcome) { o.DecisionJSON = []byte("[") }), "not a JSON object"},
		{"forged digest", tamperOutcome(func(o *episodes.Outcome) { o.DecisionSHA256 = "sha256:" + strings.Repeat("0", 64) }), "digest is invalid"},
		{"decision of another attempt", tamperDecision(func(d map[string]any) { d["attempt_id"] = "other" }), "decision attempt_id"},
		{"decision of another snapshot", tamperDecision(func(d map[string]any) { d["snapshot_digest"] = "sha256:other" }), "decision snapshot_digest"},
		{"decision without fence", tamperDecision(func(d map[string]any) { delete(d, "fence") }), "decision fence"},
	}
	requireRefusals(t, tests, executorconformance.Run)
}

func TestRunCanceledAcceptsBothWaysToReportACancellation(t *testing.T) {
	t.Parallel()
	tests := map[string]executorFunc{
		"error matching context.Canceled": func(ctx context.Context, _ *episodes.Request) (*episodes.Outcome, error) {
			return nil, ctx.Err()
		},
		"canceled outcome of the attempt": func(ctx context.Context, req *episodes.Request) (*episodes.Outcome, error) {
			return req.ContextEndingOutcome(ctx.Err(), 0), nil
		},
	}
	for name, executor := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := executorconformance.RunCanceled(t.Context(), executor); err != nil {
				t.Fatalf("RunCanceled() = %v", err)
			}
		})
	}
}

func TestRunCanceledRejectsAnExecutorThatIgnoresCancellation(t *testing.T) {
	t.Parallel()
	tests := []breach{
		{"no executor", nil, "executor is required"},
		{"produces a decision anyway", producing(nil, nil), "status"},
		{"unrelated error", executorFunc(func(context.Context, *episodes.Request) (*episodes.Outcome, error) {
			return nil, errors.New("boom")
		}), "want a context.Canceled ending"},
		{"nothing at all", executorFunc(func(context.Context, *episodes.Request) (*episodes.Outcome, error) { return nil, nil }), "neither an outcome nor an error"},
		{"timeout reported as cancellation", executorFunc(func(_ context.Context, req *episodes.Request) (*episodes.Outcome, error) {
			return req.ContextEndingOutcome(context.DeadlineExceeded, 0), nil
		}), "status"},
	}
	requireRefusals(t, tests, executorconformance.RunCanceled)
}

func TestCheckContextEndingMatchesWhatTheRunnerRecords(t *testing.T) {
	t.Parallel()
	req := executorconformance.FixtureRequest()
	tests := []struct {
		name    string
		outcome *episodes.Outcome
		ending  error
		wantErr bool
	}{
		{"cancellation", req.ContextEndingOutcome(context.Canceled, 5), context.Canceled, false},
		{"timeout", req.ContextEndingOutcome(context.DeadlineExceeded, 0), context.DeadlineExceeded, false},
		{"timeout recorded for a cancellation", req.ContextEndingOutcome(context.DeadlineExceeded, 0), context.Canceled, true},
		{"another reason", &episodes.Outcome{Status: canceledStatus, AttemptID: req.AttemptID, Fence: req.Fence, Reasons: []string{"other"}}, context.Canceled, true},
		{"another attempt", &episodes.Outcome{Status: canceledStatus, AttemptID: "other", Fence: req.Fence, Reasons: req.ContextEndingOutcome(context.Canceled, 0).Reasons}, context.Canceled, true},
		{"stale fence", &episodes.Outcome{Status: canceledStatus, AttemptID: req.AttemptID, Fence: req.Fence + 1, Reasons: req.ContextEndingOutcome(context.Canceled, 0).Reasons}, context.Canceled, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := executorconformance.CheckContextEnding(req, tt.outcome, tt.ending)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckContextEnding() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEditPayloadAndSetBudgetRewriteOnlyTheDurablePayload(t *testing.T) {
	t.Parallel()
	req := executorconformance.FixtureRequest()
	original := string(req.RequestJSON)
	executorconformance.SetBudget(req, map[string]any{"model_calls": 3})
	if string(req.RequestJSON) == original || !strings.Contains(string(req.RequestJSON), `"budget":{"model_calls":3}`) {
		t.Fatalf("budget not replaced: %s", req.RequestJSON)
	}
	if !strings.Contains(string(req.RequestJSON), `"risk_ceiling":"R1"`) {
		t.Fatalf("other payload fields lost: %s", req.RequestJSON)
	}
	executorconformance.EditPayload(req, func(payload map[string]any) { payload["kind"] = "reconsider" })
	if !strings.Contains(string(req.RequestJSON), `"kind":"reconsider"`) {
		t.Fatalf("edit not applied: %s", req.RequestJSON)
	}
}
