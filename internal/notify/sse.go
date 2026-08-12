package notify

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
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
	if r.Method != http.MethodGet {
		writeSSEProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "notification streams require GET")
		return
	}
	if cfg.DB == nil {
		writeSSEProblem(w, http.StatusServiceUnavailable, "runtime_not_ready", "notification store is not configured")
		return
	}
	tenantID := cfg.TenantID
	if cfg.TenantFromRequest != nil {
		tenantID = cfg.TenantFromRequest(r)
	}
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant")
	}
	if strings.TrimSpace(tenantID) == "" {
		writeSSEProblem(w, http.StatusBadRequest, "tenant_required", "tenant is required")
		return
	}
	if cfg.Authorize != nil && !cfg.Authorize(r, "") {
		writeSSEProblem(w, http.StatusUnauthorized, "subscriber_unauthorized", "subscriber credential is not authorized")
		return
	}
	cursor, err := requestCursor(r)
	if err != nil {
		writeSSEProblem(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeSSEProblem(w, http.StatusInternalServerError, "stream_unsupported", "response writer does not support streaming")
		return
	}
	// Prime the stream before committing headers so an expired cursor returns a
	// normal problem response and can force the client's audited resnapshot.
	page, readErr := ReadPage(r.Context(), cfg.DB, tenantID, cursor, cfg.PageSize, cfg.MaxLag, cfg.Now().UTC())
	if readErr != nil {
		writeStreamError(w, readErr)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := writeComment(w, "connected"); err != nil {
		return
	}
	flusher.Flush()

	seen := make(map[string]struct{}, cfg.PageSize)
	if err := writePage(w, flusher, r, cfg, page, &cursor, seen); err != nil {
		return
	}
	poll := time.NewTicker(cfg.PollInterval)
	defer poll.Stop()
	idle := time.NewTicker(cfg.IdleInterval)
	defer idle.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-idle.C:
			if err := writeComment(w, "idle"); err != nil {
				return
			}
			flusher.Flush()
		case <-poll.C:
			page, readErr := ReadPage(r.Context(), cfg.DB, tenantID, cursor, cfg.PageSize, cfg.MaxLag, cfg.Now().UTC())
			if readErr != nil {
				_ = writeControl(w, flusher, "stream_error", map[string]any{"code": streamErrorCode(readErr), "detail": readErr.Error()})
				return
			}
			if err := writePage(w, flusher, r, cfg, page, &cursor, seen); err != nil {
				return
			}
		}
	}
}

func requestCursor(r *http.Request) (int64, error) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("cursor")
	}
	if raw == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, fmt.Errorf("cursor must be a non-negative integer")
	}
	return cursor, nil
}

func writePage(w http.ResponseWriter, flusher http.Flusher, r *http.Request, cfg SSEConfig, page Page, cursor *int64, seen map[string]struct{}) error {
	for _, record := range page.Records {
		*cursor = record.Cursor
		key := record.Event.Source + "\x00" + record.Event.ID
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		if len(seen) >= cfg.PageSize*4 {
			clear(seen)
		}
		seen[key] = struct{}{}
		if !authorizedEvent(cfg, r, record.Event.Type) {
			continue
		}
		if err := writeEvent(w, flusher, cfg.RetryAfter, record); err != nil {
			return err
		}
	}
	if page.NextCursor > *cursor {
		*cursor = page.NextCursor
	}
	return nil
}

func authorizedEvent(cfg SSEConfig, r *http.Request, eventType string) bool {
	if len(cfg.AllowedEventTypes) > 0 {
		if _, ok := cfg.AllowedEventTypes[eventType]; !ok {
			return false
		}
	}
	return cfg.Authorize == nil || cfg.Authorize(r, eventType)
}

func writeEvent(w http.ResponseWriter, flusher http.Flusher, retry time.Duration, record Record) error {
	data, err := canonicaljson.Marshal(record.Event)
	if err != nil {
		return fmt.Errorf("marshal SSE CloudEvent: %w", err)
	}
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\nretry: %d\ndata: %s\n\n", record.Cursor, record.Event.Type, retry.Milliseconds(), data); err != nil {
		return fmt.Errorf("write SSE event: %w", err)
	}
	flusher.Flush()
	return nil
}

func writeControl(w http.ResponseWriter, flusher http.Flusher, eventType string, value map[string]any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal SSE control event: %w", err)
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
		return fmt.Errorf("write SSE control event: %w", err)
	}
	flusher.Flush()
	return nil
}

func writeComment(w http.ResponseWriter, value string) error {
	_, err := fmt.Fprintf(w, ": %s\n\n", value)
	if err != nil {
		return fmt.Errorf("write SSE comment: %w", err)
	}
	return nil
}

func writeStreamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCursorExpired):
		writeSSEProblem(w, http.StatusConflict, "cursor_expired", "cursor is outside retained notification history; perform an audited resnapshot")
	case errors.Is(err, ErrSubscriberTooSlow):
		writeSSEProblem(w, http.StatusTooManyRequests, "subscriber_too_slow", "subscriber lag exceeded the bounded backlog")
	case errors.Is(err, ErrNotificationPoison):
		writeSSEProblem(w, http.StatusServiceUnavailable, "notification_retry", "a notification failed validation and will be retried")
	default:
		writeSSEProblem(w, http.StatusInternalServerError, "notification_stream_failed", "notification stream failed")
	}
}

func streamErrorCode(err error) string {
	if errors.Is(err, ErrCursorExpired) {
		return "cursor_expired"
	}
	if errors.Is(err, ErrSubscriberTooSlow) {
		return "subscriber_too_slow"
	}
	if errors.Is(err, ErrNotificationPoison) {
		return "notification_retry"
	}
	return "notification_stream_failed"
}

func writeSSEProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "urn:agentic-stream:problem:" + code, "title": http.StatusText(status), "status": status, "code": code, "detail": detail})
}
