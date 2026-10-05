package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// TestExecutionFailureClassificationSurvivesWrapping pins that executeClaim
// may wrap executor errors: the durable attempt status and reason are derived
// from the wrapped cause, never from the message.
func TestExecutionFailureClassificationSurvivesWrapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus episodeledger.AttemptStatus
		wantReason string
	}{
		{name: "context canceled", err: context.Canceled, wantStatus: episodeledger.AttemptCancelled, wantReason: "worker_cancelled"}, //nolint:misspell // Durable protocol reason is frozen as cancelled.
		{name: "context deadline", err: context.DeadlineExceeded, wantStatus: episodeledger.AttemptTimedOut, wantReason: "worker_deadline_exceeded"},
		{name: "budget exhausted", err: &domain.BudgetExceededError{Metric: "tokens"}, wantStatus: episodeledger.AttemptFailed, wantReason: "budget_exhausted"},
		{name: "budget telemetry missing", err: domain.BudgetTelemetryMissingError{}, wantStatus: episodeledger.AttemptFailed, wantReason: "budget_telemetry_missing"},
		{name: "other", err: errors.New("boom"), wantStatus: episodeledger.AttemptFailed, wantReason: "worker_execution_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("execute episode attempt: %w", tt.err)
			if got := domain.ExecutionFailureStatus(wrapped); got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
			if got := domain.ExecutionFailureReason(wrapped); got != tt.wantReason {
				t.Fatalf("reason = %q, want %q", got, tt.wantReason)
			}
		})
	}
}

func TestTerminalAttemptStatus(t *testing.T) {
	t.Parallel()

	rejected := &decisionRecord{validationErr: errors.New("invalid")}
	tests := []struct {
		name    string
		outcome *Outcome
		record  *decisionRecord
		want    episodeledger.AttemptStatus
	}{
		{name: "valid decision", outcome: &Outcome{Status: "declined"}, record: &decisionRecord{}, want: episodeledger.AttemptProduced},
		{name: "rejected decision", outcome: &Outcome{}, record: rejected, want: episodeledger.AttemptFailed},
		{name: "no decision, no status", outcome: &Outcome{}, want: episodeledger.AttemptDeclined},
		{name: "no decision, executor status", outcome: &Outcome{Status: string(episodeledger.AttemptTimedOut)}, want: episodeledger.AttemptTimedOut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := terminalAttemptStatus(tt.outcome, tt.record); got != tt.want {
				t.Fatalf("terminalAttemptStatus = %q, want %q", got, tt.want)
			}
		})
	}
}
