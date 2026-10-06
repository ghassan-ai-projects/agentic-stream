// Package transport serves the loopback HTTP surface: liveness, readiness, epoch
// controls, approvals and the Server-Sent Events stream. Handlers translate HTTP
// to calls supplied by the caller and decide through the domain rules.
package transport

import (
	"encoding/json"
	"net/http"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"
)

// NewHealthHandler creates /health/live and /health/ready handlers. The
// handler does not expose dependency details; readiness logs/audits those at
// the owning service boundary.
func NewHealthHandler(readiness domain.Readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", serveLiveness)
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		serveReadiness(w, r, readiness)
	})
	return mux
}

func serveLiveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
}

func serveReadiness(w http.ResponseWriter, r *http.Request, readiness domain.Readiness) {
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
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(domain.Problem{
		Type: "urn:agentic-stream:problem:runtime-not-ready", Title: http.StatusText(status),
		Status: status, Detail: detail, Instance: r.URL.Path,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
