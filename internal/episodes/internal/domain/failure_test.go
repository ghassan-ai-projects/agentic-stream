package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func TestExecutionFailureIsClassifiedFromTheCauseWhetherOrNotWrapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus episodeledger.AttemptStatus
		wantReason string
	}{
		{name: "context canceled", err: context.Canceled, wantStatus: episodeledger.AttemptCancelled, wantReason: "worker_cancelled"}, //nolint:misspell // Durable protocol reason is frozen as cancelled.
		{name: "context deadline", err: context.DeadlineExceeded, wantStatus: episodeledger.AttemptTimedOut, wantReason: "worker_deadline_exceeded"},
		{name: "budget exhausted", err: &BudgetExceededError{Metric: "tokens"}, wantStatus: episodeledger.AttemptFailed, wantReason: "budget_exhausted"},
		{name: "budget telemetry missing", err: BudgetTelemetryMissingError{}, wantStatus: episodeledger.AttemptFailed, wantReason: "budget_telemetry_missing"},
		{name: "other", err: errors.New("boom"), wantStatus: episodeledger.AttemptFailed, wantReason: "worker_execution_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, err := range []error{tt.err, fmt.Errorf("execute episode attempt: %w", tt.err)} {
				if got := ExecutionFailureStatus(err); got != tt.wantStatus {
					t.Fatalf("status(%v) = %q, want %q", err, got, tt.wantStatus)
				}
				if got := ExecutionFailureReason(err); got != tt.wantReason {
					t.Fatalf("reason(%v) = %q, want %q", err, got, tt.wantReason)
				}
			}
		})
	}
}

func TestContextEndingOutcomeIsWhatTheRunnerRecordsForTheSameError(t *testing.T) {
	t.Parallel()

	req := &Request{AttemptID: "att", Fence: 3}
	both := fmt.Errorf("both: %w", errors.Join(context.DeadlineExceeded, context.Canceled))
	tests := []struct {
		name       string
		err        error
		wantStatus episodeledger.AttemptStatus
	}{
		{name: "deadline", err: context.DeadlineExceeded, wantStatus: episodeledger.AttemptTimedOut},
		{name: "cancel", err: context.Canceled, wantStatus: episodeledger.AttemptCancelled},
		{name: "wrapped deadline", err: fmt.Errorf("provider: %w", context.DeadlineExceeded), wantStatus: episodeledger.AttemptTimedOut},
		{name: "cancel wins over deadline", err: both, wantStatus: episodeledger.AttemptCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outcome := req.ContextEndingOutcome(tt.err, 7)
			if outcome.Status != string(tt.wantStatus) || outcome.Status != string(ExecutionFailureStatus(tt.err)) {
				t.Fatalf("status = %q, want %q", outcome.Status, tt.wantStatus)
			}
			if len(outcome.Reasons) != 1 || outcome.Reasons[0] != ExecutionFailureReason(tt.err) {
				t.Fatalf("reasons = %q, want [%q]", outcome.Reasons, ExecutionFailureReason(tt.err))
			}
			if outcome.AttemptID != "att" || outcome.Fence != 3 || outcome.CostMicrounits != 7 {
				t.Fatalf("outcome = %+v", outcome)
			}
			if !episodeledger.AttemptStatus(outcome.Status).CountsAsFailure() {
				t.Fatalf("%s does not consume the retry budget", outcome.Status)
			}
		})
	}
}

func TestContextEndingOutcomeExistsExactlyForContextErrors(t *testing.T) {
	t.Parallel()

	req := &Request{AttemptID: "att", Fence: 3}
	for _, err := range []error{nil, errors.New("boom"), &BudgetExceededError{Metric: "tokens"}, context.Canceled, context.DeadlineExceeded, fmt.Errorf("w: %w", context.Canceled)} {
		if (req.ContextEndingOutcome(err, 0) != nil) != (ExecutionFailureStatus(err) != episodeledger.AttemptFailed) {
			t.Errorf("%v: ContextEndingOutcome disagrees with ExecutionFailureStatus", err)
		}
	}
}
