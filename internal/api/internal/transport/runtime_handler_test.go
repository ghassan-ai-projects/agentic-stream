package transport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRuntimeHandlerMountsOnlyTheSurfacesItIsGiven(t *testing.T) {
	t.Parallel()
	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("metrics")) })
	tests := []struct {
		name       string
		build      func(t *testing.T) http.Handler
		path       string
		wantStatus int
		wantBody   string
	}{
		{"health is always mounted", func(*testing.T) http.Handler {
			return transport.NewRuntimeHandler(readiness{}, nil, transport.SSEConfig{}, nil, nil, "", "")
		}, "/health/ready", http.StatusOK, ""},
		{"metrics when supplied", func(*testing.T) http.Handler {
			return transport.NewRuntimeHandler(readiness{}, nil, transport.SSEConfig{}, metrics, nil, "", "")
		}, "/metrics", http.StatusOK, "metrics"},
		{"no metrics route without a handler", func(*testing.T) http.Handler {
			return transport.NewRuntimeHandler(readiness{}, nil, transport.SSEConfig{}, nil, nil, "", "")
		}, "/metrics", http.StatusNotFound, ""},
		{"no event route without a database", func(*testing.T) http.Handler {
			return transport.NewRuntimeHandler(readiness{}, nil, transport.SSEConfig{TenantID: "tenant"}, nil, nil, "", "")
		}, "/v1/events", http.StatusNotFound, ""},
		{"events with a database", func(t *testing.T) http.Handler {
			t.Helper()
			return transport.NewRuntimeHandler(readiness{}, storagetest.OpenTemp(t), transport.SSEConfig{}, nil, nil, "", "")
		}, "/v1/events", http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			tt.build(t).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus || tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Fatalf("GET %s = %d %q, want %d %q", tt.path, rec.Code, rec.Body.String(), tt.wantStatus, tt.wantBody)
			}
		})
	}
}
