package app_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestApprovalScopeAndOwnerRefusalsAreNonMutating(t *testing.T) {
	t.Parallel()
	ownerLost := errors.New("owner lost")
	tests := []struct {
		name, tenant string
		approvalID   func(pendingApproval) string
		owner        error
		want         error
	}{
		{name: "foreign tenant", tenant: "foreign", want: policy.ErrApprovalNotFound},
		{name: "missing request", tenant: "tenant", approvalID: func(pendingApproval) string { return "absent" }, want: policy.ErrApprovalNotFound},
		{name: "missing tenant", tenant: "", want: policy.ErrApprovalNotFound},
		{name: "owner lost", tenant: "tenant", owner: ownerLost, want: ownerLost},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			p := openPendingApproval(t)
			id := p.approvalID
			if test.approvalID != nil {
				id = test.approvalID(p)
			}
			service := newTestService(t, func(c *policy.Config) {
				if test.owner != nil {
					c.RuntimeOwner = func(context.Context, *sql.Tx, string) error { return test.owner }
				}
			})
			err := p.db.WithTx(t.Context(), func(tx *sql.Tx) error {
				_, err := service.ApprovalForSigning(t.Context(), tx, policy.ApprovalLookup{ID: id, TenantID: test.tenant, Approver: "operator-1", Relay: "relay-1", Approved: true})
				return err
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("presentation = %v, want %v", err, test.want)
			}
			err = p.db.WithTx(t.Context(), func(tx *sql.Tx) error {
				_, err := service.ResolveApproval(t.Context(), tx, policy.ApprovalResolution{ID: id, TenantID: test.tenant, Approved: true, Now: p.now})
				return err
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("resolution = %v, want %v", err, test.want)
			}
			if status := p.status(t); status != "pending" {
				t.Fatalf("a refused request changed the approval to %q", status)
			}
		})
	}
}

type principalFailure struct {
	name   string
	change func(relay, approver *string)
	sql    string
}

// principalFailures are the ways a request can name a relay or an approver
// that policy must not accept; each applies to presentation and to resolution.
var principalFailures = []principalFailure{
	{name: "the approver relays their own decision", change: func(relay, approver *string) { *relay = *approver }},
	{name: "no relay is named", change: func(relay, _ *string) { *relay = "" }},
	{name: "the relay is disabled", sql: "UPDATE principals SET status = 'disabled' WHERE principal_id = 'relay-1'"},
	{name: "the approver is disabled", sql: "UPDATE principals SET status = 'disabled' WHERE principal_id = 'operator-1'"},
	{name: "the approver's authority was revoked", sql: revokeApprovalAuthority},
}

func (f principalFailure) apply(t *testing.T, p pendingApproval, relay, approver *string) {
	t.Helper()
	if f.change != nil {
		f.change(relay, approver)
	}
	if f.sql != "" {
		exec(t, p.db, f.sql)
	}
}

func TestApprovalPresentationRequiresActiveAuthorizedPrincipals(t *testing.T) {
	t.Parallel()
	for _, failure := range principalFailures {
		t.Run(failure.name, func(t *testing.T) {
			t.Parallel()
			p := openPendingApproval(t)
			lookup := p.lookup(true)
			failure.apply(t, p, &lookup.Relay, &lookup.Approver)

			_, err := p.presentation(t, lookup)
			if !errors.Is(err, policy.ErrApprovalUnauthorized) {
				t.Fatalf("presentation = %v, want %v", err, policy.ErrApprovalUnauthorized)
			}
			if status := p.status(t); status != "pending" {
				t.Fatalf("a refused presentation changed the approval to %q", status)
			}
		})
	}
}

func TestResolvingAnApprovalRequiresActiveAuthorizedPrincipalsAndAValidSignature(t *testing.T) {
	t.Parallel()
	for _, failure := range principalFailures {
		t.Run(failure.name, func(t *testing.T) {
			t.Parallel()
			p := openPendingApproval(t)
			resolution := p.signedResolution(t, true)
			failure.apply(t, p, &resolution.Relay, &resolution.Approver)

			assertResolutionRefused(t, p, resolution)
		})
	}
	t.Run("the signature does not cover the request", func(t *testing.T) {
		t.Parallel()
		p := openPendingApproval(t)
		resolution := p.signedResolution(t, true)
		resolution.Signature = make([]byte, len(resolution.Signature))

		assertResolutionRefused(t, p, resolution)
	})
	t.Run("the decision differs from the one that was signed", func(t *testing.T) {
		t.Parallel()
		p := openPendingApproval(t)
		resolution := p.signedResolution(t, true)
		resolution.Approved = false

		assertResolutionRefused(t, p, resolution)
	})
}

func assertResolutionRefused(t *testing.T, p pendingApproval, resolution policy.ApprovalResolution) {
	t.Helper()
	if _, err := p.resolve(t, resolution); !errors.Is(err, policy.ErrApprovalUnauthorized) {
		t.Fatalf("resolution = %v, want %v", err, policy.ErrApprovalUnauthorized)
	}
	commands, outbox := commandAndOutboxCounts(t, p.db)
	bound := scalar[[]byte](t, p.db, "SELECT COALESCE(assertion_sha256, x'') FROM approvals WHERE approval_id = ?", p.approvalID)
	if status := p.status(t); status != "pending" || commands != 0 || outbox != 0 || len(bound) != 0 {
		t.Fatalf("a refused resolution left status=%q commands=%d outbox=%d assertion=%d bytes", status, commands, outbox, len(bound))
	}
}
