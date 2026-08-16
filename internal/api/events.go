package api

import (
	"encoding/json"
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
// require the bearer token in `controlToken` (a header or query value) — an
// operator action, not an anonymous kill switch.
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

type controlHandler struct {
	control *storage.EpochControl
	epoch   string
	token   string
}

func newControlHandler(control *storage.EpochControl, epoch string, token string) http.Handler {
	return &controlHandler{control: control, epoch: epoch, token: token}
}

func (h *controlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.control == nil || h.epoch == "" {
		http.Error(w, "epoch control is not configured", http.StatusServiceUnavailable)
		return
	}
	if h.token == "" || h.token != r.Header.Get("Authorization") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var err error
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/control/kill":
		err = h.control.Kill(r.Context(), h.epoch)
	case r.Method == http.MethodPost && r.URL.Path == "/control/drain":
		err = h.control.Drain(r.Context(), h.epoch)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, _ := h.control.State(r.Context(), h.epoch)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"epoch": h.epoch, "state": state})
}
