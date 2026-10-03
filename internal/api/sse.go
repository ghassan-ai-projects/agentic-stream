package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// AuthorizeSubscriber checks the subscriber credential and event type. The
// callback runs before the stream starts and for every delivered event.
type AuthorizeSubscriber func(*http.Request, string) bool

// BearerTokenAuthorizer creates a constant-time subscriber credential check.
// The token is intentionally separate from worker capability tokens.
func BearerTokenAuthorizer(expected string) AuthorizeSubscriber {
	expected = strings.TrimSpace(expected)
	return func(r *http.Request, _ string) bool {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		return expected != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
	}
}

// SSEConfig configures one durable notification stream.
type SSEConfig struct {
	DB                *storage.DB
	TenantID          string
	TenantFromRequest func(*http.Request) string
	AllowedEventTypes map[string]struct{}
	Authorize         AuthorizeSubscriber
	PageSize          int
	MaxLag            int64
	PollInterval      time.Duration
	IdleInterval      time.Duration
	RetryAfter        time.Duration
	Now               func() time.Time
}

// NewSSEHandler creates a cursor-resumable Server-Sent Events handler. The
// handler is at-least-once: clients must deduplicate by CloudEvent source/id.
func NewSSEHandler(cfg SSEConfig) http.Handler {
	if cfg.PageSize <= 0 || cfg.PageSize > 1000 {
		cfg.PageSize = 100
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.IdleInterval <= 0 {
		cfg.IdleInterval = 15 * time.Second
	}
	if cfg.RetryAfter <= 0 {
		cfg.RetryAfter = 2 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSSE(w, r, cfg)
	})
}

func serveSSE(w http.ResponseWriter, r *http.Request, cfg SSEConfig) {
	stream, ok := admitSubscriber(w, r, cfg)
	if !ok {
		return
	}
	// Prime the stream before committing headers so an expired cursor returns a
	// normal problem response and can force the client's audited resnapshot.
	page, err := stream.readPage()
	if err != nil {
		writeStreamError(w, err)
		return
	}
	if err := stream.beginResponse(); err != nil {
		return
	}
	if err := stream.deliverPage(page); err != nil {
		return
	}
	stream.followNotifications()
}

// sseStream is one subscriber's notification stream.
type sseStream struct {
	w        http.ResponseWriter
	flusher  http.Flusher
	r        *http.Request
	cfg      SSEConfig
	tenantID string
	cursor   int64
	seen     map[string]struct{}
}

// admitSubscriber requires a GET from an authorized subscriber for a known
// tenant with a valid resume cursor, answering with a problem otherwise.
func admitSubscriber(w http.ResponseWriter, r *http.Request, cfg SSEConfig) (*sseStream, bool) {
	if r.Method != http.MethodGet {
		writeSSEProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "notification streams require GET")
		return nil, false
	}
	if cfg.DB == nil {
		writeSSEProblem(w, http.StatusServiceUnavailable, "runtime_not_ready", "notification store is not configured")
		return nil, false
	}
	tenantID := subscriberTenant(r, cfg)
	if strings.TrimSpace(tenantID) == "" {
		writeSSEProblem(w, http.StatusBadRequest, "tenant_required", "tenant is required")
		return nil, false
	}
	if cfg.Authorize != nil && !cfg.Authorize(r, "") {
		writeSSEProblem(w, http.StatusUnauthorized, "subscriber_unauthorized", "subscriber credential is not authorized")
		return nil, false
	}
	cursor, err := requestCursor(r)
	if err != nil {
		writeSSEProblem(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return nil, false
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeSSEProblem(w, http.StatusInternalServerError, "stream_unsupported", "response writer does not support streaming")
		return nil, false
	}
	return &sseStream{w: w, flusher: flusher, r: r, cfg: cfg, tenantID: tenantID, cursor: cursor, seen: make(map[string]struct{}, cfg.PageSize)}, true
}

// subscriberTenant is the configured tenant, then the request's derived
// tenant, then the tenant query parameter.
func subscriberTenant(r *http.Request, cfg SSEConfig) string {
	tenantID := cfg.TenantID
	if cfg.TenantFromRequest != nil {
		tenantID = cfg.TenantFromRequest(r)
	}
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant")
	}
	return tenantID
}

func (s *sseStream) readPage() (notify.Page, error) {
	return notify.ReadPage(s.r.Context(), s.cfg.DB, s.tenantID, s.cursor, s.cfg.PageSize, s.cfg.MaxLag, s.cfg.Now().UTC()) //nolint:wrapcheck // The error detail is part of the SSE stream_error contract.
}

// beginResponse commits the event-stream headers and a connected comment.
func (s *sseStream) beginResponse() error {
	s.w.Header().Set("Content-Type", "text/event-stream")
	s.w.Header().Set("Cache-Control", "no-cache")
	s.w.Header().Set("Connection", "keep-alive")
	s.w.Header().Set("X-Accel-Buffering", "no")
	s.w.WriteHeader(http.StatusOK)
	if err := writeComment(s.w, "connected"); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *sseStream) deliverPage(page notify.Page) error {
	return writePage(s.w, s.flusher, s.r, s.cfg, page, &s.cursor, s.seen)
}

// followNotifications polls for new pages and keeps the connection alive until the
// subscriber leaves or a read fails, which ends the stream with a control
// event.
func (s *sseStream) followNotifications() {
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()
	idle := time.NewTicker(s.cfg.IdleInterval)
	defer idle.Stop()
	for {
		select {
		case <-s.r.Context().Done():
			return
		case <-idle.C:
			if err := writeComment(s.w, "idle"); err != nil {
				return
			}
			s.flusher.Flush()
		case <-poll.C:
			if err := s.pollOnce(); err != nil {
				return
			}
		}
	}
}

func (s *sseStream) pollOnce() error {
	page, err := s.readPage()
	if err != nil {
		_ = writeControl(s.w, s.flusher, "stream_error", map[string]any{"code": streamErrorCode(err), "detail": err.Error()})
		return err
	}
	return s.deliverPage(page)
}
