package app_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestHTTPApprovalAndDenialCommitOnce(t *testing.T) {
	for _, approved := range []bool{true, false} {
		name := "deny"
		if approved {
			name = "approve"
		}
		t.Run(name, func(t *testing.T) {
			f := openApprovalHTTP(t)
			body := signedApproval(t, f, approved)
			for range 2 {
				rec := approvalRequest(t, f.handler, "POST", "/v1/approvals/"+f.id, body)
				if rec.Code != 200 {
					t.Fatal(rec.Code, rec.Body.String())
				}
			}
			commands, outbox := approvalCounts(t, f.db)
			expected := 0
			if approved {
				expected = 1
			}
			if commands != expected || outbox != expected || approvalStatus(t, f) != map[bool]string{true: "approved", false: "denied"}[approved] {
				t.Fatal(commands, outbox, approvalStatus(t, f))
			}
			var events int
			if err := f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notifications WHERE event_type = 'io.agenticstream.approval.resolved.v1'").Scan(&events); err != nil {
				t.Fatal(err)
			}
			if events != 1 {
				t.Fatal("resolution events", events)
			}
			rec := approvalRequest(t, f.handler, "GET", "/v1/approvals/"+f.id+"?approver=operator-1&approved=true", nil)
			if rec.Code != 409 {
				t.Fatal(rec.Code)
			}
		})
	}
}

func TestHTTPRejectsInvalidDecisionsWithoutConsumingApproval(t *testing.T) {
	for _, change := range []string{"signature", "decision", "principal", "relay inactive", "approver inactive", "authority"} {
		t.Run(change, func(t *testing.T) {
			f := openApprovalHTTP(t)
			body := signedApproval(t, f, true)
			var input map[string]any
			if err := json.Unmarshal(body, &input); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "signature":
				input["signature"] = make([]byte, 64)
			case "decision":
				input["approved"] = false
			case "principal":
				input["approver_id"] = "relay-1"
			case "relay inactive":
				if _, err := f.db.ExecContext(t.Context(), "UPDATE principals SET status='disabled' WHERE principal_id='relay-1'"); err != nil {
					t.Fatal(err)
				}
			case "approver inactive":
				if _, err := f.db.ExecContext(t.Context(), "UPDATE principals SET status='disabled' WHERE principal_id='operator-1'"); err != nil {
					t.Fatal(err)
				}
			case "authority":
				if _, err := f.db.ExecContext(t.Context(), "DELETE FROM approval_authorities"); err != nil {
					t.Fatal(err)
				}
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			rec := approvalRequest(t, f.handler, "POST", "/v1/approvals/"+f.id, body)
			commands, outbox := approvalCounts(t, f.db)
			if rec.Code != 403 || approvalStatus(t, f) != "pending" || commands != 0 || outbox != 0 {
				t.Fatal(rec.Code, rec.Body.String(), approvalStatus(t, f), commands, outbox)
			}
			var bound []byte
			if err := f.db.QueryRowContext(t.Context(), "SELECT assertion_sha256 FROM approvals WHERE approval_id=?", f.id).Scan(&bound); err != nil {
				t.Fatal(err)
			}
			if len(bound) != 0 {
				t.Fatal("unauthorized assertion persisted")
			}
		})
	}
}

func TestHTTPResolutionRechecksCurrentStateAndTrustedClock(t *testing.T) {
	for _, change := range []string{"stale", "expired", "interlock", "health"} {
		t.Run(change, func(t *testing.T) {
			f := openApprovalHTTP(t)
			body := signedApproval(t, f, true)
			switch change {
			case "stale":
				if _, err := f.db.ExecContext(t.Context(), "UPDATE situations SET current_version=2, last_material_version=2 WHERE situation_id='sit-policy'"); err != nil {
					t.Fatal(err)
				}
			case "expired":
				f.clock.Advance(2 * time.Hour)
			case "interlock":
				if _, err := f.db.ExecContext(t.Context(), "UPDATE runtime_interlock SET status='tripped',reason='test' WHERE singleton_id=1"); err != nil {
					t.Fatal(err)
				}
			case "health":
				if _, err := f.db.ExecContext(t.Context(), "UPDATE situation_versions SET completeness='uncertain'"); err != nil {
					t.Fatal(err)
				}
			}
			rec := approvalRequest(t, f.handler, "POST", "/v1/approvals/"+f.id, body)
			commands, outbox := approvalCounts(t, f.db)
			if rec.Code != 200 || commands != 0 || outbox != 0 {
				t.Fatal(rec.Code, rec.Body.String(), commands, outbox)
			}
		})
	}
}

func TestConcurrentHTTPRepliesCreateOneCommand(t *testing.T) {
	f := openApprovalHTTP(t)
	body := signedApproval(t, f, true)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			rec := approvalRequest(t, f.handler, "POST", "/v1/approvals/"+f.id, body)
			if rec.Code != 200 {
				t.Errorf("reply: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	wg.Wait()
	commands, outbox := approvalCounts(t, f.db)
	if commands != 1 || outbox != 1 {
		t.Fatal(commands, outbox)
	}
}

func TestHTTPApprovalRollbackOnPublicationFailure(t *testing.T) {
	for _, table := range []string{"notifications", "policy_evaluations", "commands", "outbox"} {
		t.Run(table, func(t *testing.T) {
			f := openApprovalHTTP(t)
			body := signedApproval(t, f, true)
			if _, err := f.db.ExecContext(t.Context(), "CREATE TRIGGER fail_resolution BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT,'injected publication failure'); END"); err != nil {
				t.Fatal(err)
			}
			rec := approvalRequest(t, f.handler, "POST", "/v1/approvals/"+f.id, body)
			commands, outbox := approvalCounts(t, f.db)
			if rec.Code != 503 || approvalStatus(t, f) != "pending" || commands != 0 || outbox != 0 {
				t.Fatal(rec.Code, approvalStatus(t, f), commands, outbox)
			}
		})
	}
}
