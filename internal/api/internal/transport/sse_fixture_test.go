package transport_test

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const (
	sseTenant        = "tenant"
	situationEvent   = "situation.version.published"
	streamingTimeout = 10 * time.Second
)

var sseNow = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

type sseEvent struct{ id, eventType string }

func published(ids ...string) []sseEvent {
	events := make([]sseEvent, len(ids))
	for i, id := range ids {
		events[i] = sseEvent{id: id, eventType: situationEvent}
	}
	return events
}

func appendSSEEvents(t *testing.T, db *storage.DB, events ...sseEvent) {
	t.Helper()
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		for i, e := range events {
			if _, err := notify.Append(t.Context(), tx, sseTestEvent(e, sseNow.Add(time.Duration(i)*time.Second)), sseNow); err != nil {
				return fmt.Errorf("append event %s: %w", e.id, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func sseTestEvent(e sseEvent, at time.Time) contractsv1.CloudEvent {
	event := contractsv1.CloudEvent{SpecVersion: "1.0", ID: e.id, Source: "//agentic-stream/tenant/" + sseTenant, Type: e.eventType, Subject: "situation/s1", Time: at, DataContentType: "application/json", DataSchema: "urn:situation-runtime:schema:snapshot:v1", Data: map[string]any{"version": e.id}, TenantID: sseTenant, PartitionKey: "s1", IngestedTime: at, Classification: contractsv1.ClassificationInternal}
	digest, err := event.ComputeEnvelopeDigest()
	if err != nil {
		panic(err)
	}
	event.EnvelopeDigest = digest
	return event
}

func fastStream(db *storage.DB) transport.SSEConfig {
	return transport.SSEConfig{DB: db, TenantID: sseTenant, PollInterval: time.Millisecond, IdleInterval: time.Hour, Now: func() time.Time { return sseNow }}
}

type sseSession struct {
	t      *testing.T
	cancel context.CancelFunc
	body   io.ReadCloser
	reader *bufio.Reader
	ended  chan struct{}
	status int
	header http.Header
}

func connect(t *testing.T, handler http.Handler, target string, header http.Header) *sseSession {
	t.Helper()
	ended := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { ended <- struct{}{} }()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), streamingTimeout)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+target, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, values := range header {
		request.Header[name] = values
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return &sseSession{t: t, cancel: cancel, body: response.Body, reader: bufio.NewReader(response.Body), ended: ended, status: response.StatusCode, header: response.Header}
}

func (s *sseSession) nextFrame() string {
	s.t.Helper()
	var frame strings.Builder
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			s.t.Fatalf("stream ended before a complete frame (read %q): %v", frame.String(), err)
		}
		frame.WriteString(line)
		if line == "\n" {
			return frame.String()
		}
	}
}

func (s *sseSession) nextEventFrame() string {
	s.t.Helper()
	for {
		if frame := s.nextFrame(); !strings.HasPrefix(frame, ":") {
			return frame
		}
	}
}

func (s *sseSession) expectConnected() {
	s.t.Helper()
	if frame := s.nextFrame(); frame != ": connected\n\n" {
		s.t.Fatalf("first frame = %q, want the connected comment", frame)
	}
}

func (s *sseSession) expectStreamEnds() {
	s.t.Helper()
	if _, err := s.reader.ReadString('\n'); !errors.Is(err, io.EOF) {
		s.t.Fatalf("stream still open after the server ended it: %v", err)
	}
	s.expectHandlerReturned()
}

func (s *sseSession) expectHandlerReturned() {
	s.t.Helper()
	select {
	case <-s.ended:
	case <-time.After(streamingTimeout):
		s.t.Fatal("the SSE handler did not return")
	}
}

func (s *sseSession) disconnect() {
	s.t.Helper()
	s.cancel()
	_ = s.body.Close()
	s.expectHandlerReturned()
}
