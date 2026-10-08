package domain

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
)

func pendingCandidate(t *testing.T) Candidate {
	t.Helper()
	return Candidate{OutboxID: 7, OutboxStatus: OutboxPending, Command: commandRow(t), CommandStatus: actionport.CommandPending,
		Trace: contractsv1.TraceContext{Traceparent: "tp", Tracestate: "ts"}}
}

func TestAdmitDecidesEachCandidateKind(t *testing.T) {
	t.Parallel()
	expired := Lease{Owner: "crashed", Until: testNow.Add(-time.Minute), HasOwner: true, HasUntil: true}
	cases := []struct {
		name   string
		mutate func(*Candidate)
		step   AdmissionStep
		code   string
		close  string
	}{
		{"fresh command is leased", func(*Candidate) {}, AcquireLease, "", ""},
		{"expired in-flight lease is abandoned", func(c *Candidate) { c.OutboxStatus, c.Lease = OutboxLeased, expired }, AbandonExpiredLease, "", ""},
		{"lease without owner counts as expired", func(c *Candidate) { c.OutboxStatus = OutboxLeased }, AbandonExpiredLease, "", ""},
		{"succeeded command only closes outbox", func(c *Candidate) { c.CommandStatus = actionport.CommandSucceeded }, CloseOutboxOnly, "", OutboxDelivered},
		{"unknown command fails outbox", func(c *Candidate) { c.CommandStatus = actionport.CommandOutcomeUnknown }, CloseOutboxOnly, "", OutboxFailed},
		{"invalid JSON fails command", func(c *Candidate) { c.Command.JSON = []byte("{") }, FailInvalidCommand, FailureCommandJSONInvalid, ""},
		{"ambiguous document fails command", func(c *Candidate) { c.Command.JSON = contractstest.AmbiguousKeyJSON(c.Command.JSON, "command_id") }, FailInvalidCommand, FailureCommandJSONInvalid, ""},
		{"schema-invalid document fails command", func(c *Candidate) { c.Command.JSON = []byte(`{"command_id":"cmd-1"}`) }, FailInvalidCommand, FailureCommandSchemaInvalid, ""},
		{"ledger tenant mismatch fails command", func(c *Candidate) { c.Command.TenantID = "other" }, FailInvalidCommand, FailureCommandDigestMismatch, ""},
		{"digest mismatch fails command", func(c *Candidate) { c.Command.SHA = make([]byte, 32) }, FailInvalidCommand, FailureCommandDigestMismatch, ""},
		{"idempotency mismatch fails command", func(c *Candidate) { c.Command.Idempotency = make([]byte, 32) }, FailInvalidCommand, FailureCommandDigestMismatch, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			candidate := pendingCandidate(t)
			tc.mutate(&candidate)
			got := candidate.Admit(testNow)
			if got.Step != tc.step || got.FailureCode != tc.code || got.OutboxClosure != tc.close {
				t.Fatalf("admission = %+v, want step=%s code=%q close=%q", got, tc.step, tc.code, tc.close)
			}
		})
	}
}

func TestAdmitCarriesLedgerIdentityIntoLeasedCommand(t *testing.T) {
	t.Parallel()
	leased := pendingCandidate(t).Admit(testNow).Leased
	if leased.OutboxID != 7 || leased.Trace.Traceparent != "tp" || leased.Command.CommandID != "cmd-1" ||
		leased.Command.TenantID != "tenant" || leased.Command.NormalizedTarget != "motor/1" || leased.Command.IdempotencyKey != testIdempotency ||
		leased.Command.Payload["reason"] != "test" {
		t.Fatalf("leased command = %+v", leased)
	}
}

func TestAdmitRestoresLedgerIdentityOnAbandonedLease(t *testing.T) {
	t.Parallel()
	candidate := pendingCandidate(t)
	candidate.OutboxStatus = OutboxLeased
	candidate.Lease = Lease{Owner: "crashed", Until: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), HasOwner: true, HasUntil: true}
	candidate.Command.JSON = []byte("{")
	leased := candidate.Admit(testNow).Leased
	if leased.LeaseOwner != "crashed" || leased.Command.TenantID != "tenant" || leased.Command.IntentID != "int-1" {
		t.Fatalf("abandoned lease must keep ledger identity without document population: %+v", leased)
	}
}

func TestLeaseExpiredTreatsZeroAndAbsentAsExpired(t *testing.T) {
	t.Parallel()
	future := testNow.Add(time.Minute)
	cases := map[string]struct {
		lease Lease
		want  bool
	}{
		"live":              {Lease{Owner: "w", Until: future, HasOwner: true, HasUntil: true}, false},
		"exactly now":       {Lease{Owner: "w", Until: testNow, HasOwner: true, HasUntil: true}, true},
		"zero expiry":       {Lease{Owner: "w", HasOwner: true, HasUntil: true}, true},
		"missing owner":     {Lease{Until: future, HasUntil: true}, true},
		"missing expiry":    {Lease{Owner: "w", HasOwner: true}, true},
		"fractional second": {Lease{Owner: "w", Until: testNow.Add(500 * time.Millisecond), HasOwner: true, HasUntil: true}, false},
	}
	for name, tc := range cases {
		if got := tc.lease.Expired(testNow); got != tc.want {
			t.Fatalf("%s: expired = %v, want %v", name, got, tc.want)
		}
	}
}
