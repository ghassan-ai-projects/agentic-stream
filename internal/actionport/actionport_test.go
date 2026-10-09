package actionport_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestTheFacadeClassifiesAnUnknownOutcomeAndKeepsItsCause(t *testing.T) {
	t.Parallel()
	root := errors.New("provider timeout")
	unknown := fmt.Errorf("dispatch: %w", &actionport.UnknownOutcomeError{Err: root})
	if !actionport.IsUnknownOutcome(unknown) || !errors.Is(unknown, root) {
		t.Fatal("an unknown outcome must be recognized through wrapping and keep its cause")
	}
	if actionport.IsUnknownOutcome(root) {
		t.Fatal("a plain error is not an unknown outcome")
	}
}
