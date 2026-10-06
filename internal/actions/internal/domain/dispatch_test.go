package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

func TestClassifyDispatchMapsEveryLedger(t *testing.T) {
	t.Parallel()
	unknown := &actionport.UnknownOutcomeError{Err: errors.New("timeout")}
	cases := []struct {
		name   string
		effect actionport.Effect
		err    error
		want   DispatchResult
	}{
		{"success", actionport.Effect{}, nil, DispatchResult{Status: OutcomeSucceeded, Reconciliation: ReconciliationObserved, CommandStatus: CommandSucceeded, OutboxStatus: OutboxDelivered, VerificationStatus: VerificationObserved, Settled: true}},
		{"unknown", actionport.Effect{}, unknown, DispatchResult{Status: OutcomeUnknown, Reconciliation: ReconciliationRequired, CommandStatus: CommandReconciling, ErrorCode: ErrorOutcomeUnknown, OutboxStatus: OutboxFailed, VerificationStatus: VerificationAwaiting}},
		{"failure", actionport.Effect{}, errors.New("rejected"), DispatchResult{Status: OutcomeFailed, Reconciliation: ReconciliationNotRequired, CommandStatus: CommandFailed, ErrorCode: ErrorDispatchFailed, OutboxStatus: OutboxFailed, VerificationStatus: VerificationAwaiting, Settled: true}},
		{"pending verification", actionport.Effect{VerificationPending: true}, nil, DispatchResult{Status: OutcomeReconcileRequired, Reconciliation: ReconciliationRequired, CommandStatus: CommandManualReview, OutboxStatus: OutboxDelivered, VerificationStatus: VerificationAwaiting}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyDispatch(tc.effect, tc.err); got != tc.want {
				t.Fatalf("result = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNotifiedStatusHidesReconcileRequired(t *testing.T) {
	t.Parallel()
	if NotifiedStatus(OutcomeReconcileRequired) != OutcomeUnknown || NotifiedStatus(OutcomeFailed) != OutcomeFailed {
		t.Fatal("notification status mapping changed")
	}
}

func TestLeaseStandingDistinguishesForeignExpiredAndLive(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339Nano) }
	cases := []struct {
		name       string
		lease      OutboxLease
		held, live bool
	}{
		{"live", OutboxLease{OutboxLeased, "me", at(time.Minute)}, true, true},
		{"expired", OutboxLease{OutboxLeased, "me", at(-time.Second)}, true, false},
		{"unparseable expiry", OutboxLease{OutboxLeased, "me", "soon"}, true, false},
		{"foreign owner", OutboxLease{OutboxLeased, "other", at(time.Minute)}, false, false},
		{"not leased", OutboxLease{OutboxDelivered, "me", at(time.Minute)}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if held, live := tc.lease.LeaseStanding("me", now); held != tc.held || live != tc.live {
				t.Fatalf("held=%v live=%v, want %v %v", held, live, tc.held, tc.live)
			}
		})
	}
}

func TestDeviceVerificationOutcomes(t *testing.T) {
	t.Parallel()
	verifyErr := errors.New("query failed")
	check := ClassifyDeviceVerification(DeviceCheck{Effect: actionport.Effect{VerificationPending: true}, VerifyErr: verifyErr})
	if !actionport.IsUnknownOutcome(check.DispatchErr) || check.Effect.VerificationPending {
		t.Fatalf("verification error must make the dispatch unknown: %+v", check)
	}
	check = ClassifyDeviceVerification(DeviceCheck{Effect: actionport.Effect{VerificationPending: true}, FinalStatus: CommandFailed})
	if check.DispatchErr == nil || actionport.IsUnknownOutcome(check.DispatchErr) || check.Effect.VerificationPending {
		t.Fatalf("failed verification must make the dispatch failed: %+v", check)
	}
	check = ClassifyDeviceVerification(DeviceCheck{Effect: actionport.Effect{VerificationPending: true}, FinalStatus: CommandSucceeded})
	if check.DispatchErr != nil || check.Effect.VerificationPending {
		t.Fatalf("successful verification must settle the dispatch: %+v", check)
	}
	check = ClassifyDeviceVerification(DeviceCheck{Effect: actionport.Effect{VerificationPending: true}})
	if !check.Effect.VerificationPending {
		t.Fatal("a non-device command must keep its pending verification")
	}
}

func TestReconcilesUnknownNeedsUnknownDispatchAndCleanVerification(t *testing.T) {
	t.Parallel()
	unknown := &actionport.UnknownOutcomeError{Err: errors.New("timeout")}
	cases := []struct {
		name  string
		check DeviceCheck
		want  bool
	}{
		{"settled unknown", DeviceCheck{DispatchErr: unknown, FinalStatus: CommandSucceeded}, true},
		{"no final status", DeviceCheck{DispatchErr: unknown}, false},
		{"verification error", DeviceCheck{DispatchErr: unknown, FinalStatus: CommandSucceeded, VerifyErr: errors.New("x")}, false},
		{"known failure", DeviceCheck{DispatchErr: errors.New("rejected"), FinalStatus: CommandFailed}, false},
	}
	for _, tc := range cases {
		if got := tc.check.ReconcilesUnknown(); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if SkipsDeviceVerification(nil) || SkipsDeviceVerification(unknown) || !SkipsDeviceVerification(errors.New("rejected")) {
		t.Fatal("device verification applies only to success and unknown results")
	}
}

func TestOutcomeDigestBindsSchemaValidDocuments(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	document := OutcomeDocument("cmd-1", "out-1", OutcomeSucceeded, map[string]any{"accepted": true}, "", at)
	digest, err := OutcomeDigest(document)
	if err != nil || len(digest) != 32 {
		t.Fatalf("digest = %x, %v", digest, err)
	}
	if _, err := OutcomeDigest(Document{"outcome_id": "out-1"}); err == nil {
		t.Fatal("schema-invalid outcome produced a digest")
	}
	withError := OutcomeDocument("cmd-1", "out-1", OutcomeFailed, nil, ErrorDispatchFailed, at)
	if withError["error_code"] != ErrorDispatchFailed || withError["result"] != nil {
		t.Fatalf("error document = %v", withError)
	}
}
