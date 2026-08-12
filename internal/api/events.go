package api

import (
	"net/http"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// NewRuntimeHandler combines health endpoints with the durable notification
// stream exposed at /v1/events. The notification handler may be nil when a
// process only needs liveness and readiness.
func NewRuntimeHandler(readiness Readiness, db *storage.DB, events notify.SSEConfig, metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", NewHealthHandler(readiness))
	if db != nil {
		events.DB = db
		mux.Handle("/v1/events", notify.NewSSEHandler(events))
	}
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	}
	return mux
}
