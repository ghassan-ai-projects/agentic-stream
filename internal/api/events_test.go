package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const (
	controlEpoch = "epoch-1"
	controlToken = "Bearer operator-secret"
)

func newControlRuntime(t *testing.T, token string) (http.Handler, *runtimecontrol.EpochControl) {
	t.Helper()

	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	control := &runtimecontrol.EpochControl{DB: db}
	return api.NewRuntimeHandler(readiness{}, nil, api.SSEConfig{}, nil, control, controlEpoch, token), control
}

func TestControlEndpointsRequireExactToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configured    string
		authorization string
		method        string
		path          string
		wantStatus    int
		wantState     string
	}{
		{name: "kill with token", configured: controlToken, authorization: controlToken, method: http.MethodPost, path: "/control/kill", wantStatus: http.StatusOK, wantState: "killed"},
		{name: "drain with token", configured: controlToken, authorization: controlToken, method: http.MethodPost, path: "/control/drain", wantStatus: http.StatusOK, wantState: "draining"},
		{name: "missing header", configured: controlToken, method: http.MethodPost, path: "/control/kill", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", configured: controlToken, authorization: "Bearer guess", method: http.MethodPost, path: "/control/kill", wantStatus: http.StatusUnauthorized},
		{name: "token prefix only", configured: controlToken, authorization: "Bearer operator", method: http.MethodPost, path: "/control/kill", wantStatus: http.StatusUnauthorized},
		{name: "empty configured token never authorizes", configured: "", authorization: "", method: http.MethodPost, path: "/control/kill", wantStatus: http.StatusUnauthorized},
		{name: "GET is refused", configured: controlToken, authorization: controlToken, method: http.MethodGet, path: "/control/kill", wantStatus: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, control := newControlRuntime(t, tt.configured)
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			state, err := control.State(t.Context(), controlEpoch)
			if err != nil {
				t.Fatal(err)
			}
			if state != tt.wantState {
				t.Fatalf("epoch state = %q, want %q", state, tt.wantState)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["epoch"] != controlEpoch || body["state"] != tt.wantState {
				t.Fatalf("body = %v", body)
			}
		})
	}
}
