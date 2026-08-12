package api_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
)

type readiness struct{ err error }

func (r readiness) Ready() error { return r.err }

func TestHealthEndpointsAndProblemDetails(t *testing.T) {
	handler := api.NewHealthHandler(readiness{})
	for _, path := range []string{"/health/live", "/health/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
	}
	handler = api.NewHealthHandler(readiness{err: errors.New("wal recovery pending")})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("not-ready response = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	req = httptest.NewRequest(http.MethodPost, "/health/live", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST liveness status = %d", rec.Code)
	}
}
