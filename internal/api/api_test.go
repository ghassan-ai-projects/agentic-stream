package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
)

type readyNow struct{}

func (readyNow) Ready() error { return nil }

func TestFacadeBuildsTheRuntimeHandler(t *testing.T) {
	t.Parallel()
	handler := api.NewRuntimeHandler(readyNow{}, nil, api.SSEConfig{}, nil, nil, "", "")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusOK {
		t.Errorf("runtime readiness = %d", response.Code)
	}
}

func TestFacadeMountsApprovalsAndBearerAuthorization(t *testing.T) {
	t.Parallel()
	handler := api.WithApprovals(http.NotFoundHandler(), api.ApprovalConfig{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/approvals/x", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured approvals = %d, want 503", response.Code)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer secret")
	if !api.BearerTokenAuthorizer("secret")(request, "") || api.BearerTokenAuthorizer("other")(request, "") {
		t.Fatal("bearer authorizer is wrong")
	}
}
