package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
)

// CommandView is one command an intent produced, with its outcomes and
// verifications.
type CommandView = domain.CommandView

// OutcomeView is one outcome of a command as the provider reported it.
type OutcomeView = domain.OutcomeView

// VerificationView is one verification of a command's effect and its verdict.
type VerificationView = domain.VerificationView

// IntentCommands reads the commands one intent produced, with their outcomes
// and verifications.
func IntentCommands(ctx context.Context, s store.Store, intentID string) ([]domain.CommandView, error) {
	return s.IntentCommands(ctx, intentID) //nolint:wrapcheck // The store names the failed read.
}
