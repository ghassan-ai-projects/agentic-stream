package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/fixture"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRunnerExecutesAnAdmittedEpisodeAndGovernsItsDecision(t *testing.T) {
	t.Parallel()
	compiled := triggeredSpec(
		spec.Executor{Name: "fake", DispatchPolicy: "active", ModelPolicy: "test-policy", PromptVersion: "prompt-v1", Prompt: "Analyze the situation and return a typed decision."},
		spec.Intent{Type: "create_maintenance_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	)
	s := admitTriggeredSituation(t, compiled, "sit-1")
	s.assembleAndPersist(t)

	ran, err := app.NewRunner(store.New(s.db), fixture.New(), sources.Physical(), sources.Deterministic()).RunOnce(t.Context(), "default")

	if err != nil || !ran {
		t.Fatalf("RunOnce ran=%v err=%v, want true nil", ran, err)
	}
	if got := scalar[string](t, s.db, "SELECT lifecycle_status FROM episodes WHERE scheduler_item_id = ?", s.schedulerItemID); got != "concluded" {
		t.Fatalf("episode lifecycle = %q, want concluded", got)
	}
	var decisionID, validation, attemptID string
	var fence int64
	if err := s.db.QueryRowContext(t.Context(), "SELECT decision_id, validation_status, attempt_id, fence FROM decisions WHERE situation_id = ?", s.version.SituationID).
		Scan(&decisionID, &validation, &attemptID, &fence); err != nil {
		t.Fatalf("exactly one decision must be recorded: %v", err)
	}
	if validation != "accepted" || attemptID == "" || fence != 1 {
		t.Fatalf("decision provenance = status %q attempt %q fence %d, want accepted, an attempt and fence 1", validation, attemptID, fence)
	}
	if got := scalar[string](t, s.db, "SELECT status FROM episode_attempts WHERE attempt_id = ?", attemptID); got != "produced" {
		t.Fatalf("attempt status = %q, want produced", got)
	}
	var intentType, policyStatus string
	if err := s.db.QueryRowContext(t.Context(), "SELECT intent_type, policy_status FROM intents WHERE decision_id = ?", decisionID).Scan(&intentType, &policyStatus); err != nil {
		t.Fatalf("the accepted decision must leave one pending intent: %v", err)
	}
	if intentType != "create_maintenance_ticket" || policyStatus != "pending" {
		t.Fatalf("intent = type %q policy %q, want create_maintenance_ticket pending", intentType, policyStatus)
	}
}

func TestRunnerReportsNoWorkWhenNothingIsAdmitted(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	ran, err := permissiveRunner(db, fixture.New()).RunOnce(t.Context(), "default")

	if err != nil || ran {
		t.Fatalf("RunOnce ran=%v err=%v, want false nil", ran, err)
	}
}

func TestRunnerDispatchesTheOldestAcceptedEpisodeFirst(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	later := newEpisodeSeed("epi-later")
	later.AcceptedAt = "2026-08-12T10:00:00.500000000Z"
	later.insert(t, db)
	earlier := newEpisodeSeed("epi-earlier")
	earlier.AcceptedAt = "2026-08-12T10:00:00.000000000Z"
	earlier.insert(t, db)
	executor := &declinedRecorder{}

	mustRunOnce(t, permissiveRunner(db, executor))

	if len(executor.episodeIDs) != 1 || executor.episodeIDs[0] != "epi-earlier" {
		t.Fatalf("dispatched episodes = %v, want [epi-earlier]", executor.episodeIDs)
	}
}

type declinedRecorder struct{ episodeIDs []string }

func (e *declinedRecorder) Execute(_ context.Context, req *app.Request) (*app.Outcome, error) {
	e.episodeIDs = append(e.episodeIDs, req.EpisodeID)
	return &app.Outcome{Status: string(episodeledger.AttemptDeclined), AttemptID: req.AttemptID, Fence: req.Fence}, nil
}

type failingExecutor struct {
	failures int
	calls    int
}

func (e *failingExecutor) Execute(ctx context.Context, req *app.Request) (*app.Outcome, error) {
	e.calls++
	if e.calls <= e.failures {
		return nil, errors.New("transient worker failure")
	}
	outcome, err := fixture.New().Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("delegate: %w", err)
	}
	return outcome, nil
}

func TestRunnerRetriesAFailedAttemptWithTheNextFence(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-retry")
	runner := permissiveRunner(db, &failingExecutor{failures: 1})

	mustRunOnce(t, runner)

	if got := lifecycleOf(t, db, "epi-retry"); got != "running" {
		t.Fatalf("lifecycle after the failed attempt = %q, want running (kept for retry)", got)
	}
	if got := scalar[int](t, db, "SELECT COUNT(*) FROM episode_attempts WHERE episode_id = 'epi-retry' AND status = 'failed'"); got != 1 {
		t.Fatalf("failed attempts = %d, want 1", got)
	}

	mustRunOnce(t, runner)

	if got := lifecycleOf(t, db, "epi-retry"); got != "concluded" {
		t.Fatalf("lifecycle after the retry = %q, want concluded", got)
	}
	if got := scalar[int](t, db, "SELECT current_fence FROM episodes WHERE episode_id = 'epi-retry'"); got != 2 {
		t.Fatalf("episode fence = %d, want 2", got)
	}
	if got := scalar[int](t, db, "SELECT fence FROM episode_attempts WHERE episode_id = 'epi-retry' AND status = 'produced'"); got != 2 {
		t.Fatalf("produced attempt fence = %d, want 2", got)
	}
	if got := decisionCount(t, db, "epi-retry"); got != 1 {
		t.Fatalf("decisions = %d, want 1", got)
	}
}

func TestRunnerConcludesAnEpisodeAfterThreeFailedAttempts(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	seedEpisode(t, db, "epi-exhausted")
	executor := &failingExecutor{failures: 10}
	runner := permissiveRunner(db, executor)

	for range 3 {
		mustRunOnce(t, runner)
	}

	if got := lifecycleOf(t, db, "epi-exhausted"); got != "concluded" {
		t.Fatalf("lifecycle = %q, want concluded", got)
	}
	if got := terminalReasonOf(t, db, "epi-exhausted"); got != "attempt_retry_limit" {
		t.Fatalf("terminal reason = %q, want attempt_retry_limit", got)
	}
	if ran, err := runner.RunOnce(t.Context(), episodeTenant); err != nil || ran || executor.calls != 3 {
		t.Fatalf("after exhaustion RunOnce ran=%v err=%v, executor calls=%d; want false nil 3", ran, err, executor.calls)
	}
}
