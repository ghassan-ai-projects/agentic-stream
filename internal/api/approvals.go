package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
)

// ApprovalSelection identifies a request and the human decision to sign.
type ApprovalSelection struct {
	ID, Approver, Relay string
	Approved            bool
}

// ApprovalSubmission carries a signed decision; tenant and time belong to runtime.
type ApprovalSubmission struct {
	ApprovalSelection
	Signature []byte
	Reason    string
}

// ApprovalConfig binds transport callbacks to one authenticated relay.
type ApprovalConfig struct {
	Present      func(context.Context, ApprovalSelection) (any, error)
	Resolve      func(context.Context, ApprovalSubmission) (any, error)
	ErrorStatus  func(error) int
	Token, Relay string
}

type approvalHandler struct{ cfg ApprovalConfig }

// WithApprovals mounts authenticated approval operations alongside existing routes.
func WithApprovals(base http.Handler, cfg ApprovalConfig) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", base)
	mux.Handle("/v1/approvals/", &approvalHandler{cfg: cfg})
	return mux
}

func (h *approvalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if status := h.checkAccess(r); status != 0 {
		approvalProblem(w, r, status)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/approvals/")
	if id == "" || strings.Contains(id, "/") {
		approvalProblem(w, r, http.StatusNotFound)
		return
	}
	h.serveDecision(w, r, id)
}

func (h *approvalHandler) checkAccess(r *http.Request) int {
	if h.cfg.Present == nil || h.cfg.Resolve == nil || h.cfg.ErrorStatus == nil || h.cfg.Token == "" || h.cfg.Relay == "" {
		return http.StatusServiceUnavailable
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+h.cfg.Token)) != 1 {
		return http.StatusUnauthorized
	}
	return 0
}

func (h *approvalHandler) serveDecision(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		h.present(w, r, id)
	case http.MethodPost:
		h.resolve(w, r, id)
	default:
		w.Header().Set("Allow", "GET, POST")
		approvalProblem(w, r, http.StatusMethodNotAllowed)
	}
}

func (h *approvalHandler) present(w http.ResponseWriter, r *http.Request, id string) {
	approved, err := strconv.ParseBool(r.URL.Query().Get("approved"))
	if err != nil || r.URL.Query().Get("approver") == "" {
		approvalProblem(w, r, http.StatusBadRequest)
		return
	}
	result, err := h.cfg.Present(r.Context(), ApprovalSelection{ID: id, Approver: r.URL.Query().Get("approver"), Relay: h.cfg.Relay, Approved: approved})
	if err != nil {
		approvalProblem(w, r, h.cfg.ErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *approvalHandler) resolve(w http.ResponseWriter, r *http.Request, id string) {
	input, err := decodeApproval(w, r)
	if err != nil {
		approvalProblem(w, r, http.StatusBadRequest)
		return
	}
	result, err := h.cfg.Resolve(r.Context(), ApprovalSubmission{ApprovalSelection: ApprovalSelection{ID: id, Approver: input.Approver, Relay: h.cfg.Relay, Approved: *input.Approved}, Signature: input.Signature, Reason: input.Reason})
	if err != nil {
		approvalProblem(w, r, h.cfg.ErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func approvalProblem(w http.ResponseWriter, r *http.Request, status int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	writeJSONProblem(w, status, Problem{Type: "urn:agentic-stream:problem:approval", Title: http.StatusText(status), Status: status, Instance: r.URL.Path})
}
