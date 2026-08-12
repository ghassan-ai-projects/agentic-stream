package notify_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestSSEStreamsDurableEventsAndResumesFromLastEventID(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(ctx, t.TempDir()+"/sse.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for i, id := range []string{"evt-1", "evt-2"} {
		event := sseTestEvent(id, now.Add(time.Duration(i)*time.Second))
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			_, err := notify.Append(ctx, tx, event, now)
			if err != nil {
				return fmt.Errorf("append event: %w", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	handler := notify.NewSSEHandler(notify.SSEConfig{DB: db, TenantID: "tenant", PollInterval: time.Millisecond, IdleInterval: time.Hour, Now: func() time.Time { return now }})
	body := serveUntilCanceled(t, handler, "/v1/events", "")
	if !strings.Contains(body, "id: 1\nevent: situation.version.published") || !strings.Contains(body, `"id":"evt-1"`) {
		t.Fatalf("unexpected SSE body: %s", body)
	}

	resumedBody := serveUntilCanceled(t, handler, "/v1/events", "1")
	if strings.Contains(resumedBody, `"id":"evt-1"`) || !strings.Contains(resumedBody, `"id":"evt-2"`) {
		t.Fatalf("resume body=%s", resumedBody)
	}
}

func TestSSEExpiredCursorForcesAuditedResnapshot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(ctx, t.TempDir()+"/sse-expired.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	event := sseTestEvent("evt-1", now)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := notify.Append(ctx, tx, event, now)
		if err != nil {
			return fmt.Errorf("append event: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	prunedAt := now.Add(8*24*time.Hour + time.Second)
	if _, err := notify.Prune(ctx, db, prunedAt, 8*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	handler := notify.NewSSEHandler(notify.SSEConfig{DB: db, TenantID: "tenant", Now: func() time.Time { return prunedAt }})
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/events", nil)
	request.Header.Set("Last-Event-ID", "0")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "cursor_expired") {
		t.Fatalf("response=%d body=%s", response.Code, response.Body.String())
	}
	var audits int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_audits WHERE action = 'cursor_expired'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
}

func TestSSEDisconnectsSlowSubscriber(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, err := storage.Open(ctx, t.TempDir()+"/sse-slow.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for i, id := range []string{"evt-1", "evt-2"} {
		event := sseTestEvent(id, now.Add(time.Duration(i)*time.Second))
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			_, err := notify.Append(ctx, tx, event, now)
			if err != nil {
				return fmt.Errorf("append event: %w", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	handler := notify.NewSSEHandler(notify.SSEConfig{DB: db, TenantID: "tenant", MaxLag: 1, Now: func() time.Time { return now }})
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/events", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "subscriber_too_slow") {
		t.Fatalf("response=%d body=%s", response.Code, response.Body.String())
	}
}

func serveUntilCanceled(t *testing.T, handler http.Handler, path, cursor string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, path+"?tenant=tenant", nil)
	if cursor != "" {
		request.Header.Set("Last-Event-ID", cursor)
	}
	response := &cancelOnFlushWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	done := make(chan struct{})
	go func() { defer close(done); handler.ServeHTTP(response, request) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop after cancellation")
	}
	return response.Body.String()
}

type cancelOnFlushWriter struct {
	*httptest.ResponseRecorder
	cancel  context.CancelFunc
	flushes int
}

func (w *cancelOnFlushWriter) Flush() {
	w.ResponseRecorder.Flush()
	w.flushes++
	if w.flushes >= 2 {
		w.cancel()
	}
}

func sseTestEvent(id string, at time.Time) contractsv1.CloudEvent {
	event := contractsv1.CloudEvent{SpecVersion: "1.0", ID: id, Source: "//agentic-stream/tenant/tenant", Type: "situation.version.published", Subject: "situation/s1", Time: at, DataContentType: "application/json", DataSchema: "urn:situation-runtime:schema:snapshot:v1", Data: map[string]any{"version": id}, TenantID: "tenant", PartitionKey: "s1", IngestedTime: at, Classification: contractsv1.ClassificationInternal}
	digest, _ := event.ComputeEnvelopeDigest()
	event.EnvelopeDigest = digest
	return event
}
