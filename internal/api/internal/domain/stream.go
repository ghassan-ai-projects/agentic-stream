package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ParseCursor reads a resume cursor from the Last-Event-ID header or the cursor
// query parameter; empty means the beginning.
func ParseCursor(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cursor < 0 {
		return 0, fmt.Errorf("cursor must be a non-negative integer")
	}
	return cursor, nil
}

// StreamTiming is the polling, keep-alive and client-retry cadence of a stream.
type StreamTiming struct {
	PollInterval, IdleInterval, RetryAfter time.Duration
}

// WithDefaults replaces every non-positive interval with its default.
func (t StreamTiming) WithDefaults() StreamTiming {
	if t.PollInterval <= 0 {
		t.PollInterval = 500 * time.Millisecond
	}
	if t.IdleInterval <= 0 {
		t.IdleInterval = 15 * time.Second
	}
	if t.RetryAfter <= 0 {
		t.RetryAfter = 2 * time.Second
	}
	return t
}

// NormalizePageSize bounds the page size to (0, 1000], defaulting to 100.
func NormalizePageSize(size int) int {
	if size <= 0 || size > 1000 {
		return 100
	}
	return size
}

// Dedup suppresses repeated deliveries of one CloudEvent (source and id) within
// a bounded window; delivery is at-least-once, so clients deduplicate too.
type Dedup struct {
	seen  map[string]struct{}
	limit int
}

// NewDedup creates a window of four pages.
func NewDedup(pageSize int) *Dedup {
	return &Dedup{seen: make(map[string]struct{}, pageSize), limit: pageSize * 4}
}

// Repeated reports whether the event was already seen, remembering it if not.
func (d *Dedup) Repeated(source, id string) bool {
	key := source + "\x00" + id
	if _, duplicate := d.seen[key]; duplicate {
		return true
	}
	if len(d.seen) >= d.limit {
		clear(d.seen)
	}
	d.seen[key] = struct{}{}
	return false
}

// AllowedEventType reports whether the allow-list (empty means all) admits the
// type.
func AllowedEventType(allowed map[string]struct{}, eventType string) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[eventType]
	return ok
}

// EventFrame formats one CloudEvent as an SSE frame resumable by its cursor.
func EventFrame(cursor int64, eventType string, retry time.Duration, event any) (string, error) {
	data, err := canonicaljson.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal SSE CloudEvent: %w", err)
	}
	return fmt.Sprintf("id: %d\nevent: %s\nretry: %d\ndata: %s\n\n", cursor, eventType, retry.Milliseconds(), data), nil
}

// ControlFrame formats a stream control event (for example a stream error).
func ControlFrame(eventType string, value map[string]any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal SSE control event: %w", err)
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data), nil
}

// CommentFrame formats an SSE comment line (connected, idle).
func CommentFrame(value string) string { return fmt.Sprintf(": %s\n\n", value) }

// StreamFailure classifies why a notification read failed.
type StreamFailure int

// Stream failure classes.
const (
	FailureOther StreamFailure = iota
	FailureCursorExpired
	FailureSubscriberTooSlow
	FailureNotificationPoison
)

// StreamProblem is the HTTP status, stable code and detail of a stream failure.
type StreamProblem struct {
	Status       int
	Code, Detail string
}

// ProblemFor maps a failure class to its response.
func ProblemFor(failure StreamFailure) StreamProblem {
	switch failure {
	case FailureCursorExpired:
		return StreamProblem{409, "cursor_expired", "cursor is outside retained notification history; perform an audited resnapshot"}
	case FailureSubscriberTooSlow:
		return StreamProblem{429, "subscriber_too_slow", "subscriber lag exceeded the bounded backlog"}
	case FailureNotificationPoison:
		return StreamProblem{503, "notification_retry", "a notification failed validation and will be retried"}
	default:
		return StreamProblem{500, "notification_stream_failed", "notification stream failed"}
	}
}
