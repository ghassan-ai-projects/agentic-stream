package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

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

func writePage(w http.ResponseWriter, flusher http.Flusher, r *http.Request, cfg SSEConfig, page notify.Page, cursor *int64, seen map[string]struct{}) error {
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

func writeEvent(w http.ResponseWriter, flusher http.Flusher, retry time.Duration, record notify.Record) error {
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
	case errors.Is(err, notify.ErrCursorExpired):
		writeSSEProblem(w, http.StatusConflict, "cursor_expired", "cursor is outside retained notification history; perform an audited resnapshot")
	case errors.Is(err, notify.ErrSubscriberTooSlow):
		writeSSEProblem(w, http.StatusTooManyRequests, "subscriber_too_slow", "subscriber lag exceeded the bounded backlog")
	case errors.Is(err, notify.ErrNotificationPoison):
		writeSSEProblem(w, http.StatusServiceUnavailable, "notification_retry", "a notification failed validation and will be retried")
	default:
		writeSSEProblem(w, http.StatusInternalServerError, "notification_stream_failed", "notification stream failed")
	}
}

func streamErrorCode(err error) string {
	if errors.Is(err, notify.ErrCursorExpired) {
		return "cursor_expired"
	}
	if errors.Is(err, notify.ErrSubscriberTooSlow) {
		return "subscriber_too_slow"
	}
	if errors.Is(err, notify.ErrNotificationPoison) {
		return "notification_retry"
	}
	return "notification_stream_failed"
}

func writeSSEProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "urn:agentic-stream:problem:" + code, "title": http.StatusText(status), "status": status, "code": code, "detail": detail})
}
