package transport_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/transport"
)

type readiness struct{ err error }

func (r readiness) Ready() error { return r.err }

type countedReadiness struct {
	err   error
	calls int
}

func (r *countedReadiness) Ready() error { r.calls++; return r.err }

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		ready        domain.Readiness
		method, path string
		wantStatus   int
		wantBody     map[string]string
		wantDetail   string
		wantChecks   int
	}{
		{name: "liveness is true without consulting readiness", ready: &countedReadiness{err: errors.New("down")}, method: http.MethodGet, path: "/health/live", wantStatus: http.StatusOK, wantBody: map[string]string{"status": "live"}},
		{name: "ready runtime", ready: &countedReadiness{}, method: http.MethodGet, path: "/health/ready", wantStatus: http.StatusOK, wantBody: map[string]string{"status": "ready"}, wantChecks: 1},
		{name: "not ready runtime hides the dependency error", ready: &countedReadiness{err: errors.New("wal recovery pending")}, method: http.MethodGet, path: "/health/ready", wantStatus: http.StatusServiceUnavailable, wantDetail: "runtime is not ready", wantChecks: 1},
		{name: "unconfigured readiness", ready: nil, method: http.MethodGet, path: "/health/ready", wantStatus: http.StatusServiceUnavailable, wantDetail: "runtime is not configured"},
		{name: "POST liveness is refused", ready: &countedReadiness{}, method: http.MethodPost, path: "/health/live", wantStatus: http.StatusMethodNotAllowed},
		{name: "POST readiness is refused before readiness is consulted", ready: &countedReadiness{}, method: http.MethodPost, path: "/health/ready", wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown path", ready: &countedReadiness{}, method: http.MethodGet, path: "/health/other", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			transport.NewHealthHandler(tt.ready).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			assertHealthBody(t, rec, tt.wantBody, tt.wantDetail)
			if counted, ok := tt.ready.(*countedReadiness); ok && counted.calls != tt.wantChecks {
				t.Fatalf("readiness consulted %d times, want %d", counted.calls, tt.wantChecks)
			}
		})
	}
}

func assertHealthBody(t *testing.T, rec *httptest.ResponseRecorder, wantBody map[string]string, wantDetail string) {
	t.Helper()
	switch {
	case wantBody != nil:
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["status"] != wantBody["status"] || rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("body = %q (%v), content type %q; want %v as JSON", rec.Body.String(), err, rec.Header().Get("Content-Type"), wantBody)
		}
	case wantDetail != "":
		var problem domain.Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
			t.Fatalf("problem body %q: %v", rec.Body.String(), err)
		}
		if problem.Detail != wantDetail || problem.Status != rec.Code || problem.Instance != "/health/ready" || rec.Header().Get("Content-Type") != "application/problem+json" {
			t.Fatalf("problem = %+v, content type %q; want detail %q", problem, rec.Header().Get("Content-Type"), wantDetail)
		}
	}
}
