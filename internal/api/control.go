package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
)

type controlHandler struct {
	control *runtimecontrol.EpochControl
	epoch   string
	token   string
}

func newControlHandler(control *runtimecontrol.EpochControl, epoch string, token string) http.Handler {
	return &controlHandler{control: control, epoch: epoch, token: token}
}

func (h *controlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if status, detail := h.checkControlAccess(r); status != 0 {
		http.Error(w, detail, status)
		return
	}
	if status, detail := h.applyControl(r); status != 0 {
		http.Error(w, detail, status)
		return
	}
	h.writeControlState(w, r)
}

func (h *controlHandler) checkControlAccess(r *http.Request) (int, string) {
	if h.control == nil || h.epoch == "" {
		return http.StatusServiceUnavailable, "epoch control is not configured"
	}
	provided := r.Header.Get("Authorization")
	if h.token == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(h.token)) != 1 {
		return http.StatusUnauthorized, "unauthorized"
	}
	return 0, ""
}

func (h *controlHandler) applyControl(r *http.Request) (int, string) {
	var err error
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/control/kill":
		err = h.control.Kill(r.Context(), h.epoch)
	case r.Method == http.MethodPost && r.URL.Path == "/control/drain":
		err = h.control.Drain(r.Context(), h.epoch)
	default:
		return http.StatusMethodNotAllowed, "method not allowed"
	}
	if err != nil {
		return http.StatusInternalServerError, err.Error()
	}
	return 0, ""
}

func (h *controlHandler) writeControlState(w http.ResponseWriter, r *http.Request) {
	state, err := h.control.State(r.Context(), h.epoch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"epoch": h.epoch, "state": state})
}
