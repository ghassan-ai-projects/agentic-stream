package transport

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type AuthorizeSubscriber func(*http.Request, string) bool

func BearerTokenAuthorizer(expected string) AuthorizeSubscriber {
	return func(r *http.Request, _ string) bool {
		return domain.LooseBearer(r.Header.Get("Authorization"), expected)
	}
}

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

func NewSSEHandler(cfg SSEConfig) http.Handler {
	cfg.PageSize = domain.NormalizePageSize(cfg.PageSize)
	cfg = defaultStreamTiming(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveSSE(w, r, cfg)
	})
}

func serveSSE(w http.ResponseWriter, r *http.Request, cfg SSEConfig) {
	stream, ok := admitSubscriber(w, r, cfg)
	if !ok {
		return
	}
	stream.followFromCursor()
}

type sseStream struct {
	w        http.ResponseWriter
	flusher  http.Flusher
	r        *http.Request
	cfg      SSEConfig
	outbox   *notify.Service
	tenantID string
	cursor   int64
	seen     *domain.Dedup
}

func admitSubscriber(w http.ResponseWriter, r *http.Request, cfg SSEConfig) (*sseStream, bool) {
	tenantID, admitted := admitSubscriberIdentity(w, r, cfg)
	if !admitted {
		return nil, false
	}
	cursor, err := requestCursor(r)
	if err != nil {
		writeSSEProblem(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return nil, false
	}
	return openStream(w, r, cfg, tenantID, cursor)
}

func openStream(w http.ResponseWriter, r *http.Request, cfg SSEConfig, tenantID string, cursor int64) (*sseStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeSSEProblem(w, http.StatusInternalServerError, "stream_unsupported", "response writer does not support streaming")
		return nil, false
	}
	outbox, err := notify.New(cfg.DB)
	if err != nil {
		writeSSEProblem(w, http.StatusServiceUnavailable, "runtime_not_ready", "notification store is not configured")
		return nil, false
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeSSEProblem(w, http.StatusInternalServerError, "stream_unsupported", "response deadline cannot be lifted for a stream")
		return nil, false
	}
	return &sseStream{w: w, flusher: flusher, r: r, cfg: cfg, outbox: outbox, tenantID: tenantID, cursor: cursor, seen: domain.NewDedup(cfg.PageSize)}, true
}

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
	request := notify.PageRequest{TenantID: s.tenantID, Cursor: s.cursor, Limit: s.cfg.PageSize, MaxLag: s.cfg.MaxLag}
	return s.outbox.ReadPage(s.r.Context(), request, sources.NowUTC(s.cfg.Now)) //nolint:wrapcheck // The error detail is part of the SSE stream_error contract.
}

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

func (s *sseStream) followNotifications() {
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()
	idle := time.NewTicker(s.cfg.IdleInterval)
	defer idle.Stop()
	for s.followTick(poll.C, idle.C) {
	}
}

func (s *sseStream) followTick(poll, idle <-chan time.Time) bool {
	select {
	case <-s.r.Context().Done():
		return false
	case <-idle:
		return s.keepAlive() == nil
	case <-poll:
		return s.pollOnce() == nil
	}
}

func (s *sseStream) keepAlive() error {
	if err := writeComment(s.w, "idle"); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *sseStream) pollOnce() error {
	page, err := s.readPage()
	if err != nil {
		_ = writeControl(s.w, s.flusher, "stream_error", map[string]any{"code": streamErrorCode(err), "detail": err.Error()})
		return err
	}
	return s.deliverPage(page)
}

func defaultStreamTiming(cfg SSEConfig) SSEConfig {
	timing := domain.StreamTiming{PollInterval: cfg.PollInterval, IdleInterval: cfg.IdleInterval, RetryAfter: cfg.RetryAfter}.WithDefaults()
	cfg.PollInterval, cfg.IdleInterval, cfg.RetryAfter = timing.PollInterval, timing.IdleInterval, timing.RetryAfter
	return cfg
}

func (s *sseStream) followFromCursor() {

	page, err := s.readPage()
	if err != nil {
		writeStreamError(s.w, err)
		return
	}
	if err := s.beginResponse(); err != nil {
		return
	}
	if err := s.deliverPage(page); err != nil {
		return
	}
	s.followNotifications()
}

func admitSubscriberIdentity(w http.ResponseWriter, r *http.Request, cfg SSEConfig) (string, bool) {
	if !admitStreamTransport(w, r, cfg) {
		return "", false
	}
	tenantID := subscriberTenant(r, cfg)
	if strings.TrimSpace(tenantID) == "" {
		writeSSEProblem(w, http.StatusBadRequest, "tenant_required", "tenant is required")
		return "", false
	}
	if cfg.Authorize != nil && !cfg.Authorize(r, "") {
		writeSSEProblem(w, http.StatusUnauthorized, "subscriber_unauthorized", "subscriber credential is not authorized")
		return "", false
	}
	return tenantID, true
}

func admitStreamTransport(w http.ResponseWriter, r *http.Request, cfg SSEConfig) bool {
	if r.Method != http.MethodGet {
		writeSSEProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "notification streams require GET")
		return false
	}
	if cfg.DB == nil {
		writeSSEProblem(w, http.StatusServiceUnavailable, "runtime_not_ready", "notification store is not configured")
		return false
	}
	return true
}
