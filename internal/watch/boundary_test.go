package watch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestWatchPayloadValidationPrecedence(t *testing.T) {
	t.Parallel()
	command := actionport.Command{TenantID: "tenant", Payload: map[string]any{"expression": "features.x;", "target": "motor", "situation_id": "sit", "situation_version": 1, "max_fires": 1, "expires_at": "invalid"}}
	_, err := watchConditionFromCommand(command, time.Now())
	if err == nil || !strings.Contains(err.Error(), "forbidden syntax") {
		t.Fatalf("expression must precede expiry: %v", err)
	}
	command.Payload["max_fires"] = 0
	_, err = watchConditionFromCommand(command, time.Now())
	if err == nil || err.Error() != "watch condition payload is invalid" {
		t.Fatalf("identity must precede expression: %v", err)
	}
}

func TestWatchRetryWaitPreservesCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := awaitWatchRetry(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry wait = %v", err)
	}
}
