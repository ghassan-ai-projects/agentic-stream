package episodes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestExecutionFailureClassificationSurvivesWrapping pins that executeClaim
// may wrap executor errors: the durable attempt status and reason are derived
// from the wrapped cause, never from the message.
func TestExecutionFailureClassificationSurvivesWrapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus AttemptStatus
		wantReason string
	}{
		{name: "context canceled", err: context.Canceled, wantStatus: AttemptCancelled, wantReason: "worker_cancelled"},                  //nolint:misspell // Durable protocol reason is frozen as cancelled.
		{name: "grpc canceled", err: status.Error(codes.Canceled, "stop"), wantStatus: AttemptCancelled, wantReason: "worker_cancelled"}, //nolint:misspell // Durable protocol reason is frozen as cancelled.
		{name: "context deadline", err: context.DeadlineExceeded, wantStatus: AttemptTimedOut, wantReason: "worker_deadline_exceeded"},
		{name: "grpc deadline", err: status.Error(codes.DeadlineExceeded, "slow"), wantStatus: AttemptTimedOut, wantReason: "worker_deadline_exceeded"},
		{name: "budget exhausted", err: &budgetExceededError{metric: "tokens"}, wantStatus: AttemptFailed, wantReason: "budget_exhausted"},
		{name: "budget telemetry missing", err: budgetTelemetryMissingError{}, wantStatus: AttemptFailed, wantReason: "budget_telemetry_missing"},
		{name: "other", err: errors.New("boom"), wantStatus: AttemptFailed, wantReason: "worker_execution_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("execute episode attempt: %w", tt.err)
			if got := executionFailureStatus(wrapped); got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
			if got := executionFailureReason(wrapped); got != tt.wantReason {
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
		want    AttemptStatus
	}{
		{name: "valid decision", outcome: &Outcome{Status: "declined"}, record: &decisionRecord{}, want: AttemptProduced},
		{name: "rejected decision", outcome: &Outcome{}, record: rejected, want: AttemptFailed},
		{name: "no decision, no status", outcome: &Outcome{}, want: AttemptDeclined},
		{name: "no decision, executor status", outcome: &Outcome{Status: string(AttemptTimedOut)}, want: AttemptTimedOut},
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
