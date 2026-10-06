package actionport_test

import (
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestFacadeClassifiesUnknownOutcomes(t *testing.T) {
	t.Parallel()
	root := errors.New("provider timeout")
	err := &actionport.UnknownOutcomeError{Err: root}
	if !actionport.IsUnknownOutcome(err) || !errors.Is(err, root) {
		t.Fatal("an unknown outcome must be recognized and keep its cause")
	}
	if actionport.IsUnknownOutcome(root) {
		t.Fatal("a plain error is not an unknown outcome")
	}
	var command actionport.Command
	var effect actionport.Effect
	if command.CommandID != "" || effect.VerificationPending {
		t.Fatal("zero values must be empty")
	}
}
