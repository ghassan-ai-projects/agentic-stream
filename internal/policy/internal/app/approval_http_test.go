package app_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestHTTPApprovalAndDenialCommitOnce(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		approved     bool
		wantStatus   string
		wantCommands int
	}{
		{"approve", true, "approved", 1},
		{"deny", false, "denied", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := openApprovalHTTP(t)
			body := signedApproval(t, f, test.approved)
			for range 2 {
				if rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body); rec.Code != http.StatusOK {
					t.Fatalf("POST = %d %s", rec.Code, rec.Body.String())
				}
			}
			commands, outbox := commandAndOutboxCounts(t, f.db)
			resolved := scalar[int](t, f.db, "SELECT COUNT(*) FROM notifications WHERE event_type = 'io.agenticstream.approval.resolved.v1'")
			if status := f.status(t); status != test.wantStatus || commands != test.wantCommands || outbox != test.wantCommands || resolved != 1 {
				t.Fatalf("status=%q commands=%d outbox=%d resolution events=%d, want %s/%d/%d/1", status, commands, outbox, resolved, test.wantStatus, test.wantCommands, test.wantCommands)
			}
			if rec := approvalRequest(t, f.handler, http.MethodGet, "/v1/approvals/"+f.approvalID+"?approver=operator-1&approved=true", nil); rec.Code != http.StatusConflict {
				t.Fatalf("presenting a resolved approval = %d, want %d", rec.Code, http.StatusConflict)
			}
		})
	}
}

func TestHTTPRefusesAnUnauthorizedDecisionWithoutConsumingTheApproval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		zeroSignature bool
		sql           string
	}{
		{name: "the signature does not cover the request", zeroSignature: true},
		{name: "the approver's authority was revoked", sql: revokeApprovalAuthority},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := openApprovalHTTP(t)
			var input map[string]any
			if err := json.Unmarshal(signedApproval(t, f, true), &input); err != nil {
				t.Fatal(err)
			}
			if test.zeroSignature {
				input["signature"] = make([]byte, 64)
			}
			if test.sql != "" {
				exec(t, f.db, test.sql)
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}

			rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body)
			commands, outbox := commandAndOutboxCounts(t, f.db)
			if rec.Code != http.StatusForbidden || f.status(t) != "pending" || commands != 0 || outbox != 0 {
				t.Fatalf("POST = %d %s, status %q commands=%d outbox=%d; want 403 and nothing consumed", rec.Code, rec.Body.String(), f.status(t), commands, outbox)
			}
		})
	}
}

func TestHTTPResolutionUsesTheRuntimeClockNotTheCallers(t *testing.T) {
	t.Parallel()
	f := openApprovalHTTP(t)
	body := signedApproval(t, f, true)
	f.clock.Advance(2 * time.Hour)

	rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body)
	commands, outbox := commandAndOutboxCounts(t, f.db)
	if rec.Code != http.StatusOK || f.status(t) != "expired" || commands != 0 || outbox != 0 {
		t.Fatalf("POST = %d %s, status %q commands=%d outbox=%d; want the approval expired by the runtime clock", rec.Code, rec.Body.String(), f.status(t), commands, outbox)
	}
}

func TestConcurrentHTTPRepliesCreateOneCommand(t *testing.T) {
	t.Parallel()
	f := openApprovalHTTP(t)
	body := signedApproval(t, f, true)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body)
			if rec.Code != http.StatusOK {
				t.Errorf("reply: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	wg.Wait()
	if commands, outbox := commandAndOutboxCounts(t, f.db); commands != 1 || outbox != 1 {
		t.Fatalf("commands=%d outbox=%d after concurrent replies, want 1/1", commands, outbox)
	}
}

func TestHTTPApprovalRollsBackWhenAnyPublicationWriteFails(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"notifications", "policy_evaluations", "commands", "outbox"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			f := openApprovalHTTP(t)
			body := signedApproval(t, f, true)
			exec(t, f.db, "CREATE TRIGGER fail_resolution BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT,'injected publication failure'); END")

			rec := approvalRequest(t, f.handler, http.MethodPost, "/v1/approvals/"+f.approvalID, body)
			commands, outbox := commandAndOutboxCounts(t, f.db)
			if rec.Code != http.StatusServiceUnavailable || f.status(t) != "pending" || commands != 0 || outbox != 0 {
				t.Fatalf("POST = %d, status %q commands=%d outbox=%d; want 503 with the approval still pending", rec.Code, f.status(t), commands, outbox)
			}
		})
	}
}
