package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func TestLiveUDSSourceQuarantinesMalformedLinesAndCountsValidLines(t *testing.T) {
	db := storagetest.OpenTemp(t)

	runtimeTelemetry := telemetry.NewRuntime(time.Unix(1, 0))
	source := newLiveService(t, db, runtimeTelemetry)
	ctx := context.Background()
	var received contractsv1.Envelope
	malformed := domain.LiveLine{ConnectionID: 1, LineNumber: 1, Data: []byte("not-json\n")}
	if err := source.processLine(ctx, "inst", malformed, func(_ context.Context, _ contractsv1.Envelope) error {
		t.Fatal("malformed line reached the sink")
		return nil
	}); err != nil {
		t.Fatalf("process malformed line: %v", err)
	}

	envelope := contractsv1.Envelope{
		ID: "evt-live-1", Type: "motor.vibration.observed", SchemaVersion: "1.0", TenantID: "default",
		Source: "gateway", PartitionKey: "motor-1", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-1"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"rms_mm_s": 1.0},
	}
	line, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.processLine(ctx, "inst", domain.LiveLine{ConnectionID: 1, LineNumber: 2, Data: line}, func(_ context.Context, got contractsv1.Envelope) error {
		received = got
		return nil
	}); err != nil {
		t.Fatalf("process valid line: %v", err)
	}
	if received.ID != envelope.ID {
		t.Fatalf("received event = %q, want %q", received.ID, envelope.ID)
	}
	if got := runtimeTelemetry.Snapshot()["agentic_stream_live_lines_ingested_total"]; got != 1 {
		t.Fatalf("live lines ingested = %d, want 1", got)
	}
	if got := runtimeTelemetry.Snapshot()["agentic_stream_live_lines_rejected_total"]; got != 1 {
		t.Fatalf("live lines rejected = %d, want 1", got)
	}
	var raw string
	if err := db.QueryRowContext(ctx, "SELECT json_extract(payload_json, '$.data.raw') FROM event_quarantine WHERE reason_code = 'malformed_json'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "not-json\n" {
		t.Fatalf("quarantined raw line = %q, want original line", raw)
	}
}

func TestLiveUDSSourceAcceptsReconnects(t *testing.T) {
	db := storagetest.OpenTemp(t)

	path := filepath.Join("/tmp", fmt.Sprintf("agentic-stream-live-%d.sock", time.Now().UnixNano()))
	source := newLiveService(t, db, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	received := make(chan string, 2)
	runDone := make(chan error, 1)
	go func() {
		runDone <- source.ServeLive(ctx, path, func(ctx context.Context, env contractsv1.Envelope) error {
			if _, err := source.log.Append(ctx, "default", []contractsv1.Envelope{env}); err != nil {
				return fmt.Errorf("append test event: %w", err)
			}
			received <- env.ID
			return nil
		})
	}()

	// The socket file appears at bind, before the listener accepts, so wait
	// until a dial succeeds rather than until the file exists.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if probe, dialErr := (&net.Dialer{}).DialContext(context.Background(), "unix", path); dialErr == nil {
			_ = probe.Close()
			break
		}
		select {
		case runErr := <-runDone:
			if runErr != nil && (errors.Is(runErr, syscall.EPERM) || strings.Contains(runErr.Error(), "operation not permitted")) {
				t.Skipf("Unix socket listeners unavailable: %v", runErr)
			}
			t.Fatalf("live source stopped before listening: %v", runErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("live source did not create its socket")
		}
		time.Sleep(time.Millisecond)
	}

	writeMalformed := func() {
		t.Helper()
		conn, dialErr := (&net.Dialer{}).DialContext(context.Background(), "unix", path)
		if dialErr != nil {
			t.Fatal(dialErr)
		}
		defer func() { _ = conn.Close() }()
		if _, writeErr := conn.Write([]byte("not-json\n")); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	writeEvent := func(id string) {
		t.Helper()
		conn, dialErr := (&net.Dialer{}).DialContext(context.Background(), "unix", path)
		if dialErr != nil {
			t.Fatal(dialErr)
		}
		defer func() { _ = conn.Close() }()
		env := contractsv1.Envelope{
			ID: id, Type: "motor.vibration.observed", SchemaVersion: "1.0", TenantID: "default", Source: "gateway",
			PartitionKey: "motor-1", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-1"},
			EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
			Classification: contractsv1.ClassificationInternal, Data: map[string]any{"rms_mm_s": 1.0},
		}
		line, marshalErr := json.Marshal(env)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, writeErr := conn.Write(append(line, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	writeMalformed()
	writeEvent("evt-live-1")
	writeEvent("evt-live-2")
	for expected := 0; expected < 2; expected++ {
		select {
		case <-received:
		case <-time.After(time.Second):
			t.Fatal("live source did not deliver a reconnecting client event")
		}
	}
	cancel()
	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Fatalf("live source shutdown: %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("live source did not shut down")
	}
}

func TestLiveUDSSourcePropagatesSinkDeadlineWithActiveParent(t *testing.T) {
	db := storagetest.OpenTemp(t)

	path := filepath.Join("/tmp", fmt.Sprintf("agentic-stream-live-deadline-%d.sock", time.Now().UnixNano()))
	source := newLiveService(t, db, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() {
		runDone <- source.ServeLive(ctx, path, func(context.Context, contractsv1.Envelope) error {
			return context.DeadlineExceeded
		})
	}()

	// The socket file appears at bind, before the listener accepts, so wait
	// until a dial succeeds rather than until the file exists.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if probe, dialErr := (&net.Dialer{}).DialContext(context.Background(), "unix", path); dialErr == nil {
			_ = probe.Close()
			break
		}
		select {
		case runErr := <-runDone:
			t.Fatalf("live source stopped before listening: %v", runErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("live source did not create its socket")
		}
		time.Sleep(time.Millisecond)
	}

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	envelope := contractsv1.Envelope{
		ID: "evt-live-deadline", Type: "motor.vibration.observed", SchemaVersion: "1.0", TenantID: "default", Source: "gateway",
		PartitionKey: "motor-1", Entity: contractsv1.EntityRef{Type: "motor", ID: "motor-1"},
		EventTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), IngestedAt: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Classification: contractsv1.ClassificationInternal, Data: map[string]any{"rms_mm_s": 1.0},
	}
	line, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	select {
	case runErr := <-runDone:
		if runErr == nil || !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("sink deadline result = %v, want propagated deadline", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("live source did not surface sink deadline")
	}
}

func newLiveService(t *testing.T, db *storage.DB, runtimeTelemetry *telemetry.Runtime) *Service {
	t.Helper()
	cfg := Config{Store: store.New(db), Log: eventlog.NewEventLog(db), TenantID: "default", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if runtimeTelemetry != nil {
		cfg.Telemetry = runtimeTelemetry
	}
	service, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestOversizedLiveLineIsQuarantinedAsItsBoundedPrefix(t *testing.T) {
	db := storagetest.OpenTemp(t)

	source := newLiveService(t, db, nil)
	prefix := bytes.Repeat([]byte("x"), domain.MaxLineBytes)
	item := domain.LiveLine{ConnectionID: 7, LineNumber: 1, Data: prefix, ReadErr: domain.ErrLineTooLarge}
	if err := source.processLine(context.Background(), "inst", item, func(context.Context, contractsv1.Envelope) error {
		t.Fatal("oversized line reached the sink")
		return nil
	}); err != nil {
		t.Fatalf("process oversized line: %v", err)
	}
	var raw string
	if err := db.QueryRowContext(context.Background(), "SELECT json_extract(payload_json, '$.data.raw') FROM event_quarantine WHERE reason_code = 'line_too_large'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != string(prefix) {
		t.Fatalf("quarantined raw prefix = %d bytes, want %d", len(raw), len(prefix))
	}
}
