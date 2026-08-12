// Package api provides the small, loopback-safe HTTP surface for runtime
// liveness and readiness. Business operations remain in internal packages.
package api

import (
	"encoding/json"
	"net/http"
)

// Readiness reports whether the runtime can safely accept work.
type Readiness interface {
	Ready() error
}

// Problem is RFC 9457 Problem Details for HTTP API failures.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// NewHealthHandler creates /health/live and /health/ready handlers. The
// handler does not expose dependency details; readiness logs/audits those at
// the owning service boundary.
func NewHealthHandler(readiness Readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if readiness == nil {
			writeProblem(w, r, http.StatusServiceUnavailable, "runtime is not configured")
			return
		}
		if err := readiness.Ready(); err != nil {
			writeProblem(w, r, http.StatusServiceUnavailable, "runtime is not ready")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	return mux
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type: "urn:agentic-stream:problem:runtime-not-ready", Title: http.StatusText(status),
		Status: status, Detail: detail, Instance: r.URL.Path,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
