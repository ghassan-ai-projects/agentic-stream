package actions

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// CommandView is one command an intent produced, with its outcomes and
// verifications.
type CommandView = app.CommandView

// OutcomeView is one outcome of a command as the provider reported it.
type OutcomeView = app.OutcomeView

// VerificationView is one verification of a command's effect and its verdict.
type VerificationView = app.VerificationView

// IntentCommands reads the commands one intent produced, with their outcomes
// and verifications. It only reads.
func IntentCommands(ctx context.Context, db *storage.DB, intentID string) ([]CommandView, error) {
	return app.IntentCommands(ctx, store.Reader(db), intentID)
}
