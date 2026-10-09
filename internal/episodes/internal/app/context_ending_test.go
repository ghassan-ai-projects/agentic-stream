package app_test

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRunnerRecordsAContextEndingTheSameWhetherReturnedAsOutcomeOrError(t *testing.T) {
	t.Parallel()
	endings := []struct {
		name   string
		err    error
		status episodeledger.AttemptStatus
	}{
		{"deadline", context.DeadlineExceeded, episodeledger.AttemptTimedOut},
		{"cancel", context.Canceled, episodeledger.AttemptCancelled},
	}
	for _, ending := range endings {
		returns := map[string]app.Executor{
			"outcome": executorFunc(func(_ context.Context, req *app.Request) (*app.Outcome, error) {
				return req.ContextEndingOutcome(ending.err, 0), nil
			}),
			"error": executorFunc(func(context.Context, *app.Request) (*app.Outcome, error) { return nil, ending.err }),
		}
		for kind, executor := range returns {
			t.Run(ending.name+" as "+kind, func(t *testing.T) {
				t.Parallel()
				db := storagetest.OpenTemp(t)
				seedEpisode(t, db, "epi-ending")

				mustRunOnce(t, runnerWithEpochGate(db, executor))

				if got := attemptStatusOf(t, db, "epi-ending"); got != string(ending.status) {
					t.Fatalf("attempt status = %q, want %q", got, ending.status)
				}
			})
		}
	}
}
