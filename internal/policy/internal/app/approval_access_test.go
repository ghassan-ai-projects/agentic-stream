package app_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestApprovalScopeAndOwnerRefusalsAreNonMutating(t *testing.T) {
	f := openApprovalHTTP(t)
	ownerErr := errors.New("owner lost")
	for _, tc := range []struct {
		name, tenant, id     string
		owner, errorExpected error
	}{
		{"foreign tenant", "foreign", f.id, nil, policy.ErrApprovalNotFound},
		{"missing request", "tenant", "absent", nil, policy.ErrApprovalNotFound},
		{"missing tenant", "", f.id, nil, policy.ErrApprovalNotFound},
		{"owner lost", "tenant", f.id, ownerErr, ownerErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestService(t, func(c *policy.Config) {
				if tc.owner != nil {
					c.RuntimeOwner = func(context.Context, *sql.Tx, string) error { return tc.owner }
				}
			})
			err := f.db.WithTx(t.Context(), func(tx *sql.Tx) error {
				_, err := service.ApprovalForSigning(t.Context(), tx, policy.ApprovalLookup{ID: tc.id, TenantID: tc.tenant, Approver: "operator-1", Relay: "relay-1", Approved: true})
				return err
			})
			if !errors.Is(err, tc.errorExpected) {
				t.Fatal(err)
			}
			err = f.db.WithTx(t.Context(), func(tx *sql.Tx) error {
				_, err := service.ResolveApproval(t.Context(), tx, policy.ApprovalResolution{ID: tc.id, TenantID: tc.tenant, Approved: true, Now: f.clock.Now()})
				return err
			})
			if !errors.Is(err, tc.errorExpected) {
				t.Fatal(err)
			}
			if approvalStatus(t, f) != "pending" {
				t.Fatal("refused request changed approval")
			}
		})
	}
}

func TestApprovalPresentationRequiresActiveAuthorizedPrincipals(t *testing.T) {
	for _, change := range []string{"same principal", "no relay", "inactive relay", "inactive approver", "no authority"} {
		t.Run(change, func(t *testing.T) {
			f := openApprovalHTTP(t)
			request := policy.ApprovalLookup{ID: f.id, TenantID: "tenant", Approver: "operator-1", Relay: "relay-1", Approved: true}
			switch change {
			case "same principal":
				request.Relay = request.Approver
			case "no relay":
				request.Relay = ""
			case "inactive relay":
				if _, err := f.db.ExecContext(t.Context(), "UPDATE principals SET status='disabled' WHERE principal_id='relay-1'"); err != nil {
					t.Fatal(err)
				}
			case "inactive approver":
				if _, err := f.db.ExecContext(t.Context(), "UPDATE principals SET status='disabled' WHERE principal_id='operator-1'"); err != nil {
					t.Fatal(err)
				}
			case "no authority":
				if _, err := f.db.ExecContext(t.Context(), "DELETE FROM approval_authorities"); err != nil {
					t.Fatal(err)
				}
			}
			service := newTestService(t)
			err := f.db.WithTx(t.Context(), func(tx *sql.Tx) error { _, err := service.ApprovalForSigning(t.Context(), tx, request); return err })
			if !errors.Is(err, policy.ErrApprovalUnauthorized) || approvalStatus(t, f) != "pending" {
				t.Fatal(err)
			}
		})
	}
}

func TestCommittedApprovalDispatchesWithoutNewSensorInput(t *testing.T) {
	f := openApprovalHTTP(t)
	body := signedApproval(t, f, true)
	rec := approvalRequest(t, f.handler, "POST", "/v1/approvals/"+f.id, body)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if err := f.pipeline.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := f.db.QueryRowContext(t.Context(), "SELECT status FROM outbox WHERE kind='command'").Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "delivered" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("committed outbox was not delivered without sensor input")
}
