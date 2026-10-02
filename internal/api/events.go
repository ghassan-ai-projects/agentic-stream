package api

import (
	"net/http"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// NewRuntimeHandler combines health endpoints with the durable notification
// stream exposed at /v1/events. The notification handler may be nil when a
// process only needs liveness and readiness.
//
// P8: when control is non-nil the handler exposes /control/drain and
// /control/kill for the CURRENT policy epoch. A killed epoch refuses every
// later decision; a drained epoch refuses only new admission. The endpoints
// require the Authorization header to equal `controlToken` exactly, compared
// in constant time — an operator action, not an anonymous kill switch.
func NewRuntimeHandler(readiness Readiness, db *storage.DB, events notify.SSEConfig, metrics http.Handler, control *storage.EpochControl, epoch string, controlToken string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", NewHealthHandler(readiness))
	if db != nil {
		events.DB = db
		mux.Handle("/v1/events", notify.NewSSEHandler(events))
	}
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	}
	if control != nil {
		handler := newControlHandler(control, epoch, controlToken)
		mux.Handle("/control/drain", handler)
		mux.Handle("/control/kill", handler)
	}
	return mux
}
