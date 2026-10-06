package transport_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
)

type countedReadiness struct{ calls int }

func (r *countedReadiness) Ready() error { r.calls++; return nil }

func TestHealthRejectsMethodBeforeConsultingReadiness(t *testing.T) {
	t.Parallel()
	ready := &countedReadiness{}
	handler := api.NewHealthHandler(ready)
	for _, path := range []string{"/health/live", "/health/ready"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
	}
	if ready.calls != 0 {
		t.Fatalf("method rejection called readiness %d times", ready.calls)
	}
	for _, path := range []string{"/health/live", "/health/ready"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	}
	if ready.calls != 1 {
		t.Fatalf("GET endpoints called readiness %d times", ready.calls)
	}
}

func TestUnconfiguredHealthPreservesProblemResponse(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	api.NewHealthHandler(nil).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Content-Type") != "application/problem+json" || !strings.Contains(rec.Body.String(), `"detail":"runtime is not configured"`) {
		t.Fatalf("unconfigured readiness response: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestControlConfigurationPrecedesAuthenticationAndMethod(t *testing.T) {
	t.Parallel()
	handler := api.NewRuntimeHandler(readiness{}, nil, api.SSEConfig{}, nil, &runtimecontrol.EpochControl{}, "", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/control/kill", nil))
	if rec.Code != http.StatusServiceUnavailable || rec.Body.String() != "epoch control is not configured\n" {
		t.Fatalf("configuration precedence: %d %q", rec.Code, rec.Body.String())
	}
}

func TestControlAuthenticationPrecedesMethodAndStateChanges(t *testing.T) {
	t.Parallel()
	handler, control := newControlRuntime(t, controlToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/control/kill", nil))
	if rec.Code != http.StatusUnauthorized || rec.Body.String() != "unauthorized\n" {
		t.Fatalf("authentication precedence: %d %q", rec.Code, rec.Body.String())
	}
	if state, err := control.State(t.Context(), controlEpoch); err != nil || state != "" {
		t.Fatalf("rejected control mutated state: state=%q err=%v", state, err)
	}
}

func TestControlReturnsPersistenceFailureWithoutSuccessBody(t *testing.T) {
	t.Parallel()
	handler, control := newControlRuntime(t, controlToken)
	if err := control.DB.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/control/kill", nil)
	req.Header.Set("Authorization", controlToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Header().Get("Content-Type"), "application/json") || !strings.Contains(rec.Body.String(), "database is closed") {
		t.Fatalf("persistence failure response: %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
}
