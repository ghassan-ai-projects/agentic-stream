package transport_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/api"
	"github.com/ghassan-ai-projects/agentic-stream/internal/api/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSSEAdmissionPreservesProblemPrecedence(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	tests := []struct {
		name, method, tenant, cursor, code string
		store                              *storage.DB
		authorized                         bool
		status                             int
	}{
		{"method before store", http.MethodPost, "", "-1", "method_not_allowed", nil, false, 405},
		{"store before tenant", http.MethodGet, "", "-1", "runtime_not_ready", nil, false, 503},
		{"tenant before authorization", http.MethodGet, "", "-1", "tenant_required", db, false, 400},
		{"authorization before cursor", http.MethodGet, "tenant", "-1", "subscriber_unauthorized", db, false, 401},
		{"invalid cursor", http.MethodGet, "tenant", "-1", "invalid_cursor", db, true, 400},
		{"non-numeric cursor", http.MethodGet, "tenant", "latest", "invalid_cursor", db, true, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := transport.NewSSEHandler(transport.SSEConfig{DB: tt.store, TenantID: tt.tenant, Authorize: func(*http.Request, string) bool { return tt.authorized }})
			request := httptest.NewRequestWithContext(t.Context(), tt.method, "/v1/events", nil)
			request.Header.Set("Last-Event-ID", tt.cursor)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assertStreamProblem(t, response, tt.status, tt.code)
		})
	}
}

func assertStreamProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var problem struct {
		Type   string `json:"type"`
		Code   string `json:"code"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("problem body %q: %v", response.Body.String(), err)
	}
	if response.Code != status || problem.Code != code || problem.Status != status || problem.Type != "urn:agentic-stream:problem:"+code || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("response = %d %v %s; want %d %s as problem+json", response.Code, response.Header(), response.Body.String(), status, code)
	}
}

func TestSSEAnswersARefusedResumeWithAProblemBeforeAnyStream(t *testing.T) {
	t.Parallel()
	t.Run("an expired cursor forces an audited resnapshot", func(t *testing.T) {
		t.Parallel()
		db := storagetest.OpenTemp(t)
		appendSSEEvents(t, db, published("evt-1")...)
		prunedAt := sseNow.Add(8*24*time.Hour + time.Second)
		outbox, err := notify.New(db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := outbox.Prune(t.Context(), prunedAt, 8*24*time.Hour); err != nil {
			t.Fatal(err)
		}
		handler := transport.NewSSEHandler(transport.SSEConfig{DB: db, TenantID: sseTenant, Now: func() time.Time { return prunedAt }})
		response := serveSSEOnce(t, handler, "0")
		assertStreamProblem(t, response, http.StatusConflict, "cursor_expired")
		var audits int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM notification_audits WHERE action = 'cursor_expired'").Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("audits = %d, %v; want 1", audits, err)
		}
	})
	t.Run("a subscriber too far behind is refused", func(t *testing.T) {
		t.Parallel()
		db := storagetest.OpenTemp(t)
		appendSSEEvents(t, db, published("evt-1", "evt-2")...)
		handler := transport.NewSSEHandler(transport.SSEConfig{DB: db, TenantID: sseTenant, MaxLag: 1, Now: func() time.Time { return sseNow }})
		assertStreamProblem(t, serveSSEOnce(t, handler, ""), http.StatusTooManyRequests, "subscriber_too_slow")
	})
}

func serveSSEOnce(t *testing.T, handler http.Handler, lastEventID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/events", nil)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestSSEStreamsDurableEventsInFramesAndResumesAfterTheCursor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, target string
		header       http.Header
		wantIDs      []string
	}{
		{"from the beginning", "/v1/events?tenant=tenant", nil, []string{"1", "2"}},
		{"after the Last-Event-ID header", "/v1/events?tenant=tenant", http.Header{"Last-Event-Id": {"1"}}, []string{"2"}},
		{"after the cursor query parameter", "/v1/events?tenant=tenant&cursor=1", nil, []string{"2"}},
		{"the header wins over the query", "/v1/events?tenant=tenant&cursor=2", http.Header{"Last-Event-Id": {"0"}}, []string{"1", "2"}},
		{"past the last event", "/v1/events?tenant=tenant&cursor=2", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			appendSSEEvents(t, db, published("evt-1", "evt-2")...)
			cfg := fastStream(db)
			cfg.TenantID = ""
			session := connect(t, transport.NewSSEHandler(cfg), tt.target, tt.header)
			if session.status != http.StatusOK || session.header.Get("Content-Type") != "text/event-stream" || session.header.Get("Cache-Control") != "no-cache" || session.header.Get("X-Accel-Buffering") != "no" {
				t.Fatalf("stream response = %d %v", session.status, session.header)
			}
			session.expectConnected()
			for _, id := range tt.wantIDs {
				frame := session.nextFrame()
				if !strings.HasPrefix(frame, "id: "+id+"\nevent: "+situationEvent+"\nretry: 2000\ndata: {") || !strings.Contains(frame, `"id":"evt-`+id+`"`) || !strings.HasSuffix(frame, "}\n\n") {
					t.Fatalf("frame for cursor %s = %q", id, frame)
				}
			}
			session.disconnect()
		})
	}
}

func TestSSEDeliversEventsAppendedAfterTheSubscriberConnected(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	appendSSEEvents(t, db, published("evt-seed")...)
	cfg := fastStream(db)
	cfg.RetryAfter = 3 * time.Second
	session := connect(t, transport.NewSSEHandler(cfg), "/v1/events", nil)
	session.expectConnected()
	if frame := session.nextFrame(); !strings.Contains(frame, `"id":"evt-seed"`) {
		t.Fatalf("seed frame = %q", frame)
	}
	appendSSEEvents(t, db, published("evt-late")...)
	frame := session.nextEventFrame()
	if !strings.HasPrefix(frame, "id: 2\nevent: "+situationEvent+"\nretry: 3000\ndata: ") || !strings.Contains(frame, `"id":"evt-late"`) {
		t.Fatalf("live frame = %q", frame)
	}
	session.disconnect()
}

func TestSSEKeepsAnIdleConnectionAliveWithComments(t *testing.T) {
	t.Parallel()
	cfg := fastStream(storagetest.OpenTemp(t))
	cfg.IdleInterval = time.Millisecond
	session := connect(t, transport.NewSSEHandler(cfg), "/v1/events", nil)
	session.expectConnected()
	if frame := session.nextFrame(); frame != ": idle\n\n" {
		t.Fatalf("idle frame = %q", frame)
	}
	session.disconnect()
}

func TestSSEStopsWhenTheClientDisconnects(t *testing.T) {
	t.Parallel()
	session := connect(t, transport.NewSSEHandler(fastStream(storagetest.OpenTemp(t))), "/v1/events", nil)
	session.expectConnected()
	session.disconnect()
}

func TestSSEShowsEachTenantOnlyItsOwnEvents(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	appendSSEEvents(t, db, published("evt-1")...)
	cfg := fastStream(db)
	cfg.TenantID = ""
	cfg.IdleInterval = time.Millisecond
	handler := transport.NewSSEHandler(cfg)
	other := connect(t, handler, "/v1/events?tenant=other", nil)
	other.expectConnected()
	if frame := other.nextFrame(); frame != ": idle\n\n" {
		t.Fatalf("another tenant's first frame = %q, want idle: nothing may leak across tenants", frame)
	}
	other.disconnect()
	own := connect(t, handler, "/v1/events?tenant=tenant", nil)
	own.expectConnected()
	if frame := own.nextFrame(); !strings.Contains(frame, `"id":"evt-1"`) {
		t.Fatalf("own tenant's first frame = %q", frame)
	}
	own.disconnect()
}

func TestSSEResolvesTheTenantFromTheRequestWhenTheDeploymentProvidesOne(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	appendSSEEvents(t, db, published("evt-1")...)
	cfg := fastStream(db)
	cfg.TenantID = "ignored"
	cfg.TenantFromRequest = func(r *http.Request) string { return r.Header.Get("X-Tenant") }
	session := connect(t, transport.NewSSEHandler(cfg), "/v1/events", http.Header{"X-Tenant": {sseTenant}})
	session.expectConnected()
	if frame := session.nextFrame(); !strings.Contains(frame, `"id":"evt-1"`) {
		t.Fatalf("first frame = %q", frame)
	}
	session.disconnect()
}

func TestSSEDeliversOnlyEventTypesTheSubscriberMayRead(t *testing.T) {
	t.Parallel()
	events := []sseEvent{{"evt-secret", "secret.type"}, {"evt-public", "public.type"}, {"evt-secret-2", "secret.type"}}
	tests := []struct {
		name  string
		apply func(*transport.SSEConfig)
	}{
		{"allow-list", func(c *transport.SSEConfig) { c.AllowedEventTypes = map[string]struct{}{"public.type": {}} }},
		{"per-event authorization", func(c *transport.SSEConfig) {
			c.Authorize = func(_ *http.Request, eventType string) bool { return eventType == "" || eventType == "public.type" }
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			appendSSEEvents(t, db, events...)
			cfg := fastStream(db)
			tt.apply(&cfg)
			session := connect(t, transport.NewSSEHandler(cfg), "/v1/events", nil)
			session.expectConnected()
			frame := session.nextFrame()
			if !strings.HasPrefix(frame, "id: 2\nevent: public.type\n") {
				t.Fatalf("first delivered frame = %q, want the public event at cursor 2", frame)
			}
			appendSSEEvents(t, db, sseEvent{"evt-public-2", "public.type"})
			if frame := session.nextEventFrame(); !strings.HasPrefix(frame, "id: 4\nevent: public.type\n") {
				t.Fatalf("next delivered frame = %q, want the next public event at cursor 4, skipping the secret one", frame)
			}
			session.disconnect()
		})
	}
}

func TestSSEEndsWithAStreamErrorWhenAFollowUpReadFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      func(*storage.DB) transport.SSEConfig
		provoke  func(t *testing.T, db *storage.DB, round int)
		wantCode string
	}{
		{"storage failure", fastStream, func(t *testing.T, db *storage.DB, _ int) {
			t.Helper()
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		}, "notification_stream_failed"},
		{"subscriber falls too far behind", func(db *storage.DB) transport.SSEConfig {
			cfg := fastStream(db)
			cfg.MaxLag = 1
			return cfg
		}, func(t *testing.T, db *storage.DB, round int) {
			t.Helper()
			appendSSEEvents(t, db, published(fmt.Sprintf("evt-lag-%d-a", round), fmt.Sprintf("evt-lag-%d-b", round))...)
		}, "subscriber_too_slow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			appendSSEEvents(t, db, published("evt-1")...)
			session := connect(t, transport.NewSSEHandler(tt.cfg(db)), "/v1/events", nil)
			session.expectConnected()
			session.nextFrame()
			frame := provokeUntilStreamError(t, session, db, tt.provoke)
			if !strings.HasPrefix(frame, "event: stream_error\ndata: {") || strings.Contains(frame, "id:") || !strings.Contains(frame, `"code":"`+tt.wantCode+`"`) {
				t.Fatalf("control frame = %q, want stream_error %s without an id", frame, tt.wantCode)
			}
			session.expectStreamEnds()
		})
	}
}

// provokeUntilStreamError repeats the provocation while a poll that straddled
// the commit delivered the new events instead of seeing the lag: the outbox
// reads its bounds and its rows in two separate queries, so one poll can miss
// the lag a commit created. A later round outpaces the subscriber again.
func provokeUntilStreamError(t *testing.T, session *sseSession, db *storage.DB, provoke func(*testing.T, *storage.DB, int)) string {
	t.Helper()
	const maxRounds = 50
	for round := 1; round <= maxRounds; round++ {
		provoke(t, db, round)
		if frame := session.nextEventFrame(); strings.HasPrefix(frame, "event: stream_error") {
			return frame
		}
	}
	t.Fatalf("no stream_error after %d provocations", maxRounds)
	return ""
}

func TestSSEStopsWhenTheConnectionCannotBeWrittenTo(t *testing.T) {
	t.Parallel()
	for _, writesBeforeFailure := range []int{0, 1} {
		t.Run(map[int]string{0: "connected comment", 1: "first event"}[writesBeforeFailure], func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			appendSSEEvents(t, db, published("evt-1")...)
			w := &brokenConnection{header: http.Header{}, writesBeforeFailure: writesBeforeFailure}
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/events", nil)
			transport.NewSSEHandler(fastStream(db)).ServeHTTP(w, request)
			if w.writes != writesBeforeFailure+1 {
				t.Fatalf("writes = %d, want the handler to stop after the failed write number %d", w.writes, writesBeforeFailure+1)
			}
		})
	}
}

type brokenConnection struct {
	header              http.Header
	writesBeforeFailure int
	writes              int
}

func (b *brokenConnection) Header() http.Header { return b.header }
func (b *brokenConnection) WriteHeader(int)     {}
func (b *brokenConnection) Flush()              {}
func (b *brokenConnection) Write(p []byte) (int, error) {
	b.writes++
	if b.writes > b.writesBeforeFailure {
		return 0, http.ErrHandlerTimeout
	}
	return len(p), nil
}

type unflushableWriter struct {
	header http.Header
	status int
	body   strings.Builder
}

func (u *unflushableWriter) Header() http.Header { return u.header }
func (u *unflushableWriter) WriteHeader(s int)   { u.status = s }
func (u *unflushableWriter) Write(p []byte) (int, error) {
	return u.body.Write(p)
}

func TestSSERefusesAWriterThatCannotStream(t *testing.T) {
	t.Parallel()
	w := &unflushableWriter{header: http.Header{}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/events", nil)
	transport.NewSSEHandler(fastStream(storagetest.OpenTemp(t))).ServeHTTP(w, request)
	if w.status != http.StatusInternalServerError || !strings.Contains(w.body.String(), `"code":"stream_unsupported"`) {
		t.Fatalf("response = %d %s", w.status, w.body.String())
	}
}

func TestBearerTokenAuthorizerComparesTheSubscriberCredential(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, header, expected string
		want                   bool
	}{
		{"missing header", "", "subscriber-secret", false},
		{"wrong token", "Bearer wrong", "subscriber-secret", false},
		{"valid token", "Bearer subscriber-secret", "subscriber-secret", true},
		{"valid token with surrounding space", "Bearer  subscriber-secret ", "subscriber-secret", true},
		{"no configured token authorizes nobody", "Bearer ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/events", nil)
			if tt.header != "" {
				request.Header.Set("Authorization", tt.header)
			}
			if got := api.BearerTokenAuthorizer(tt.expected)(request, ""); got != tt.want {
				t.Fatalf("authorized = %v, want %v", got, tt.want)
			}
		})
	}
}
