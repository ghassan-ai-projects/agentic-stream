package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
)

type CommandView = domain.CommandView

type OutcomeView = domain.OutcomeView

type VerificationView = domain.VerificationView

func IntentCommands(ctx context.Context, s store.Store, intentID string) ([]domain.CommandView, error) {
	commands, err := s.IntentCommands(ctx, intentID)
	if err != nil {
		return nil, fmt.Errorf("commands of intent %s: %w", intentID, err)
	}
	return commands, nil
}
