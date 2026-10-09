package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

type contextEndingOutcomeExecutor struct{ ending error }

func (e contextEndingOutcomeExecutor) Execute(_ context.Context, req *app.Request) (*app.Outcome, error) {
	return req.ContextEndingOutcome(e.ending, 0), nil
}

type contextEndingErrorExecutor struct{ ending error }

func (e contextEndingErrorExecutor) Execute(context.Context, *app.Request) (*app.Outcome, error) {
	return nil, e.ending
}

func TestRunnerRecordsAContextEndingTheSameWhetherReturnedAsOutcomeOrError(t *testing.T) {
	for _, ending := range []struct {
		name   string
		err    error
		status episodeledger.AttemptStatus
	}{
		{name: "deadline", err: context.DeadlineExceeded, status: episodeledger.AttemptTimedOut},
		{name: "cancel", err: context.Canceled, status: episodeledger.AttemptCancelled},
	} {
		for name, executor := range map[string]app.Executor{
			"outcome": contextEndingOutcomeExecutor{ending: ending.err},
			"error":   contextEndingErrorExecutor{ending: ending.err},
		} {
			t.Run(ending.name+" as "+name, func(t *testing.T) {
				db := storagetest.OpenTemp(t)
				seedFreshnessEpisode(t, db, "epi-ending", "sit-ending", 1, 1)
				runner := withEpochRunner(db, executor, sources.Physical(), sources.Deterministic())
				if _, err := runner.RunOnce(context.Background(), "tenant"); err != nil {
					t.Fatal(err)
				}
				var status string
				if err := db.QueryRowContext(context.Background(), "SELECT status FROM episode_attempts WHERE episode_id = 'epi-ending'").Scan(&status); err != nil {
					t.Fatal(err)
				}
				if status != string(ending.status) {
					t.Fatalf("attempt status = %q, want %q", status, ending.status)
				}
			})
		}
	}
}
