package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/transport"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const (
	controlEpoch = "epoch-1"
	controlToken = "Bearer operator-secret"
)

func newControlRuntime(t *testing.T, token string) (http.Handler, *runtimecontrol.EpochControl) {
	t.Helper()
	control := &runtimecontrol.EpochControl{DB: storagetest.OpenTemp(t)}
	return api.NewRuntimeHandler(readiness{}, nil, transport.SSEConfig{}, nil, control, controlEpoch, token), control
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
			rec := serveControl(t, handler, tt.method, tt.path, tt.authorization)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if state, err := control.State(t.Context(), controlEpoch); err != nil || state != tt.wantState {
				t.Fatalf("epoch state = %q (%v), want %q", state, err, tt.wantState)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["epoch"] != controlEpoch || body["state"] != tt.wantState || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("body = %v, content type %q", body, rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestControlConfigurationPrecedesAuthenticationAndMethod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		control *runtimecontrol.EpochControl
		epoch   string
	}{
		{"no epoch", &runtimecontrol.EpochControl{}, ""},
		{"no control", nil, controlEpoch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := api.NewRuntimeHandler(readiness{}, nil, transport.SSEConfig{}, nil, tt.control, tt.epoch, "")
			rec := serveControl(t, handler, http.MethodGet, "/control/kill", "")
			if tt.control == nil {
				if rec.Code != http.StatusNotFound {
					t.Fatalf("control routes without a control = %d, want 404", rec.Code)
				}
				return
			}
			if rec.Code != http.StatusServiceUnavailable || rec.Body.String() != "epoch control is not configured\n" {
				t.Fatalf("configuration precedence: %d %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestControlAuthenticationPrecedesMethodAndStateChanges(t *testing.T) {
	t.Parallel()
	handler, control := newControlRuntime(t, controlToken)
	rec := serveControl(t, handler, http.MethodGet, "/control/kill", "")
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
	rec := serveControl(t, handler, http.MethodPost, "/control/kill", controlToken)
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Content-Type") == "application/json" || !strings.Contains(rec.Body.String(), "database is closed") {
		t.Fatalf("persistence failure response: %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
}

func serveControl(t *testing.T, handler http.Handler, method, path, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
