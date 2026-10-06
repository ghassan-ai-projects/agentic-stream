package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func requestCursor(r *http.Request) (int64, error) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("cursor")
	}
	return domain.ParseCursor(raw) //nolint:wrapcheck // The cursor message is part of the problem detail.
}

func writePage(w http.ResponseWriter, flusher http.Flusher, r *http.Request, cfg SSEConfig, page notify.Page, cursor *int64, seen *domain.Dedup) error {
	for _, record := range page.Records {
		if err := deliverRecord(w, flusher, r, cfg, record, cursor, seen); err != nil {
			return err
		}
	}
	if page.NextCursor > *cursor {
		*cursor = page.NextCursor
	}
	return nil
}

func deliverRecord(w http.ResponseWriter, flusher http.Flusher, r *http.Request, cfg SSEConfig, record notify.Record, cursor *int64, seen *domain.Dedup) error {
	*cursor = record.Cursor
	if seen.Repeated(record.Event.Source, record.Event.ID) {
		return nil
	}
	if !authorizedEvent(cfg, r, record.Event.Type) {
		return nil
	}
	return writeEvent(w, flusher, cfg.RetryAfter, record)
}

func authorizedEvent(cfg SSEConfig, r *http.Request, eventType string) bool {
	return domain.AllowedEventType(cfg.AllowedEventTypes, eventType) && (cfg.Authorize == nil || cfg.Authorize(r, eventType))
}

func writeEvent(w http.ResponseWriter, flusher http.Flusher, retry time.Duration, record notify.Record) error {
	frame, err := domain.EventFrame(record.Cursor, record.Event.Type, retry, record.Event)
	if err != nil {
		return fmt.Errorf("frame SSE event: %w", err)
	}
	return writeFrame(w, flusher, frame, "SSE event")
}

func writeControl(w http.ResponseWriter, flusher http.Flusher, eventType string, value map[string]any) error {
	frame, err := domain.ControlFrame(eventType, value)
	if err != nil {
		return fmt.Errorf("frame SSE control event: %w", err)
	}
	return writeFrame(w, flusher, frame, "SSE control event")
}

func writeFrame(w http.ResponseWriter, flusher http.Flusher, frame, what string) error {
	if _, err := io.WriteString(w, frame); err != nil {
		return fmt.Errorf("write %s: %w", what, err)
	}
	flusher.Flush()
	return nil
}

func writeComment(w http.ResponseWriter, value string) error {
	if _, err := io.WriteString(w, domain.CommentFrame(value)); err != nil {
		return fmt.Errorf("write SSE comment: %w", err)
	}
	return nil
}

func writeStreamError(w http.ResponseWriter, err error) {
	problem := domain.ProblemFor(classify(err))
	writeSSEProblem(w, problem.Status, problem.Code, problem.Detail)
}

func streamErrorCode(err error) string { return domain.ProblemFor(classify(err)).Code }

// classify names the failure class of a notification read error.
func classify(err error) domain.StreamFailure {
	switch {
	case errors.Is(err, notify.ErrCursorExpired):
		return domain.FailureCursorExpired
	case errors.Is(err, notify.ErrSubscriberTooSlow):
		return domain.FailureSubscriberTooSlow
	case errors.Is(err, notify.ErrNotificationPoison):
		return domain.FailureNotificationPoison
	default:
		return domain.FailureOther
	}
}

func writeSSEProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "urn:agentic-stream:problem:" + code, "title": http.StatusText(status), "status": status, "code": code, "detail": detail})
}
