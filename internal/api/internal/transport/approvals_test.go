package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"
)

func TestApprovalTransportRejectsInvalidInputsBeforeResolution(t *testing.T) {
	signature := strings.Repeat("A", 86) + "=="
	valid := `{"approver_id":"human","approved":false,"signature":"` + signature + `","reason":"reviewed"}`
	for _, tc := range []struct {
		name, method, path, token, body string
		status                          int
	}{
		{"missing token", "POST", "/v1/approvals/id", "", valid, 401},
		{"wrong token", "POST", "/v1/approvals/id", "Bearer wrong", valid, 401},
		{"method", "DELETE", "/v1/approvals/id", "Bearer token", valid, 405},
		{"missing id", "GET", "/v1/approvals/", "Bearer token", "", 404},
		{"nested id", "GET", "/v1/approvals/id/extra", "Bearer token", "", 404},
		{"decision missing", "GET", "/v1/approvals/id?approver=human", "Bearer token", "", 400},
		{"principal missing", "GET", "/v1/approvals/id?approved=true", "Bearer token", "", 400},
		{"malformed", "POST", "/v1/approvals/id", "Bearer token", "{", 400},
		{"unknown fields", "POST", "/v1/approvals/id", "Bearer token", strings.TrimSuffix(valid, "}") + `,"tenant_id":"other"}`, 400},
		{"trailing object", "POST", "/v1/approvals/id", "Bearer token", valid + "{}", 400},
		{"trailing garbage", "POST", "/v1/approvals/id", "Bearer token", valid + "garbage", 400},
		{"missing decision", "POST", "/v1/approvals/id", "Bearer token", strings.ReplaceAll(valid, `"approved":false,`, ""), 400},
		{"missing reason", "POST", "/v1/approvals/id", "Bearer token", strings.ReplaceAll(valid, `"reviewed"`, `" "`), 400},
		{"short signature", "POST", "/v1/approvals/id", "Bearer token", strings.ReplaceAll(valid, signature, "AA=="), 400},
		{"too large", "POST", "/v1/approvals/id", "Bearer token", strings.ReplaceAll(valid, "reviewed", strings.Repeat("x", 20<<10)), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cfg := domain.ApprovalConfig{Token: "token", Relay: "relay", ErrorStatus: func(error) int { return 503 }, Present: func(context.Context, domain.ApprovalSelection) (any, error) { calls++; return nil, nil }, Resolve: func(context.Context, domain.ApprovalSubmission) (any, error) { calls++; return nil, nil }}
			h := WithApprovals(NewHealthHandler(nil), cfg)
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", tc.token)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.status || calls != 0 {
				t.Fatal(rec.Code, calls, rec.Body.String())
			}
			if rec.Header().Get("Content-Type") != "application/problem+json" || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(rec.Header())
			}
		})
	}
}

func TestApprovalTransportBindsRelayAndRoutesFailures(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			cfg := domain.ApprovalConfig{Token: "token", Relay: "registered-relay", ErrorStatus: func(error) int { return 403 }}
			check := func(r domain.ApprovalSelection) {
				if r.ID != "id" || r.Approver != "human" || r.Relay != "registered-relay" || r.Approved {
					t.Fatal(r)
				}
			}
			cfg.Present = func(_ context.Context, r domain.ApprovalSelection) (any, error) {
				check(r)
				return nil, errors.New("secret database details")
			}
			cfg.Resolve = func(_ context.Context, r domain.ApprovalSubmission) (any, error) {
				check(r.ApprovalSelection)
				return nil, errors.New("secret database details")
			}
			body, _ := json.Marshal(map[string]any{"approver_id": "human", "approved": false, "signature": make([]byte, 64), "reason": "reviewed"})
			req := httptest.NewRequestWithContext(t.Context(), method, "/v1/approvals/id?approver=human&approved=false", strings.NewReader(string(body)))
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			WithApprovals(NewHealthHandler(nil), cfg).ServeHTTP(rec, req)
			if rec.Code != 403 || strings.Contains(rec.Body.String(), "secret") {
				t.Fatal(rec.Code, rec.Body.String())
			}
		})
	}
}

func TestApprovalTransportFailsClosedWithoutConfiguration(t *testing.T) {
	rec := httptest.NewRecorder()
	WithApprovals(NewHealthHandler(nil), domain.ApprovalConfig{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), "GET", "/v1/approvals/id", nil))
	if rec.Code != 503 {
		t.Fatal(rec.Code)
	}
	rec = httptest.NewRecorder()
	WithApprovals(NewHealthHandler(nil), domain.ApprovalConfig{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), "GET", "/health/live", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}
