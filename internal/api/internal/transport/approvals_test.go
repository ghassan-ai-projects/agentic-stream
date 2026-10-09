package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/transport"
)

var approvalSignature = strings.Repeat("A", 86) + "=="

const approvalToken = "Bearer token"

func validApprovalBody() string {
	return `{"approver_id":"human","approved":false,"signature":"` + approvalSignature + `","reason":"reviewed"}`
}

type approvalCalls struct{ presented, resolved []string }

func recordingApprovalConfig(calls *approvalCalls) domain.ApprovalConfig {
	return domain.ApprovalConfig{
		Token: "token", Relay: "registered-relay", ErrorStatus: func(error) int { return http.StatusForbidden },
		Present: func(_ context.Context, r domain.ApprovalSelection) (any, error) {
			calls.presented = append(calls.presented, r.ID+"/"+r.Approver+"/"+r.Relay)
			return map[string]string{"presentation": r.ID}, nil
		},
		Resolve: func(_ context.Context, r domain.ApprovalSubmission) (any, error) {
			calls.resolved = append(calls.resolved, r.ID+"/"+r.Approver+"/"+r.Relay+"/"+r.Reason)
			return map[string]string{"resolved": r.ID}, nil
		},
	}
}

func serveApproval(t *testing.T, cfg domain.ApprovalConfig, method, target, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	req.Header.Set("Authorization", authorization)
	rec := httptest.NewRecorder()
	transport.WithApprovals(transport.NewHealthHandler(nil), cfg).ServeHTTP(rec, req)
	return rec
}

func TestApprovalsAreRejectedBeforeAnyCallbackRuns(t *testing.T) {
	t.Parallel()
	valid := validApprovalBody()
	tests := []struct {
		name, method, target, authorization, body string
		status                                    int
		allow                                     string
	}{
		{"missing token", "POST", "/v1/approvals/id", "", valid, 401, ""},
		{"wrong token", "POST", "/v1/approvals/id", "Bearer wrong", valid, 401, ""},
		{"method", "DELETE", "/v1/approvals/id", approvalToken, valid, 405, "GET, POST"},
		{"missing id", "GET", "/v1/approvals/", approvalToken, "", 404, ""},
		{"nested id", "GET", "/v1/approvals/id/extra", approvalToken, "", 404, ""},
		{"decision missing", "GET", "/v1/approvals/id?approver=human", approvalToken, "", 400, ""},
		{"decision unparseable", "GET", "/v1/approvals/id?approver=human&approved=maybe", approvalToken, "", 400, ""},
		{"principal missing", "GET", "/v1/approvals/id?approved=true", approvalToken, "", 400, ""},
		{"malformed", "POST", "/v1/approvals/id", approvalToken, "{", 400, ""},
		{"unknown fields", "POST", "/v1/approvals/id", approvalToken, strings.TrimSuffix(valid, "}") + `,"tenant_id":"other"}`, 400, ""},
		{"trailing object", "POST", "/v1/approvals/id", approvalToken, valid + "{}", 400, ""},
		{"trailing garbage", "POST", "/v1/approvals/id", approvalToken, valid + "garbage", 400, ""},
		{"missing decision", "POST", "/v1/approvals/id", approvalToken, strings.ReplaceAll(valid, `"approved":false,`, ""), 400, ""},
		{"missing reason", "POST", "/v1/approvals/id", approvalToken, strings.ReplaceAll(valid, `"reviewed"`, `" "`), 400, ""},
		{"short signature", "POST", "/v1/approvals/id", approvalToken, strings.ReplaceAll(valid, approvalSignature, "AA=="), 400, ""},
		{"too large", "POST", "/v1/approvals/id", approvalToken, strings.ReplaceAll(valid, "reviewed", strings.Repeat("x", 20<<10)), 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls approvalCalls
			rec := serveApproval(t, recordingApprovalConfig(&calls), tt.method, tt.target, tt.authorization, tt.body)
			if rec.Code != tt.status || len(calls.presented)+len(calls.resolved) != 0 {
				t.Fatalf("status = %d, callbacks = %+v, body = %s; want %d and none", rec.Code, calls, rec.Body.String(), tt.status)
			}
			assertApprovalProblem(t, rec, tt.status)
			if got := rec.Header().Get("Allow"); got != tt.allow {
				t.Fatalf("Allow = %q, want %q", got, tt.allow)
			}
		})
	}
}

func assertApprovalProblem(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	var problem domain.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("problem body %q: %v", rec.Body.String(), err)
	}
	if problem.Status != status || problem.Title != http.StatusText(status) || problem.Type != "urn:agentic-stream:problem:approval" {
		t.Fatalf("problem = %+v, want status %d", problem, status)
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestApprovalsPresentAndResolveBoundToTheRegisteredRelay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, method, target, body string
		wantResult                 string
		wantCalls                  func(approvalCalls) []string
		want                       string
	}{
		{"present", http.MethodGet, "/v1/approvals/id?approver=human&approved=false&relay=attacker", "", `{"presentation":"id"}`, func(c approvalCalls) []string { return c.presented }, "id/human/registered-relay"},
		{"resolve", http.MethodPost, "/v1/approvals/id", validApprovalBody(), `{"resolved":"id"}`, func(c approvalCalls) []string { return c.resolved }, "id/human/registered-relay/reviewed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls approvalCalls
			rec := serveApproval(t, recordingApprovalConfig(&calls), tt.method, tt.target, approvalToken, tt.body)
			if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != tt.wantResult || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("response = %d %q (%q), want 200 %s", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"), tt.wantResult)
			}
			if got := tt.wantCalls(calls); len(got) != 1 || got[0] != tt.want {
				t.Fatalf("callback saw %v, want %q", got, tt.want)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestApprovalFailuresMapToTheConfiguredStatusWithoutLeakingDetail(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			var calls approvalCalls
			cfg := recordingApprovalConfig(&calls)
			failure := errors.New("secret database details")
			cfg.Present = func(context.Context, domain.ApprovalSelection) (any, error) { return nil, failure }
			cfg.Resolve = func(context.Context, domain.ApprovalSubmission) (any, error) { return nil, failure }
			target := "/v1/approvals/id?approver=human&approved=false"
			rec := serveApproval(t, cfg, method, target, approvalToken, validApprovalBody())
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "secret") {
				t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
			}
			assertApprovalProblem(t, rec, http.StatusForbidden)
		})
	}
}

func TestApprovalsFailClosedUntilEveryPartIsConfigured(t *testing.T) {
	t.Parallel()
	var calls approvalCalls
	complete := recordingApprovalConfig(&calls)
	tests := map[string]domain.ApprovalConfig{
		"empty":         {},
		"no token":      {Present: complete.Present, Resolve: complete.Resolve, ErrorStatus: complete.ErrorStatus, Relay: "relay"},
		"no relay":      {Present: complete.Present, Resolve: complete.Resolve, ErrorStatus: complete.ErrorStatus, Token: "token"},
		"no error code": {Present: complete.Present, Resolve: complete.Resolve, Token: "token", Relay: "relay"},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := serveApproval(t, cfg, http.MethodGet, "/v1/approvals/id?approver=human&approved=true", approvalToken, "")
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			assertApprovalProblem(t, rec, http.StatusServiceUnavailable)
		})
	}
	t.Run("health stays reachable", func(t *testing.T) {
		t.Parallel()
		rec := serveApproval(t, domain.ApprovalConfig{}, http.MethodGet, "/health/live", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("liveness beside unconfigured approvals = %d", rec.Code)
		}
	})
}
