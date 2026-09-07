package ingress

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/telemetry"
)

func TestLiveUDSSourceQuarantinesMalformedLinesAndCountsValidLines(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	runtimeTelemetry := telemetry.NewRuntime(time.Unix(1, 0))
	source := NewLiveUDSSource(eventlog.NewEventLog(db), "default", "/tmp/live.sock").
		WithTelemetry(runtimeTelemetry).
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	var received contractsv1.Envelope
	malformed := liveLine{connectionID: 1, lineNumber: 1, data: []byte("not-json\n")}
	if err := source.processLine(ctx, malformed, func(_ context.Context, _ contractsv1.Envelope) error {
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
	if err := source.processLine(ctx, liveLine{connectionID: 1, lineNumber: 2, data: line}, func(_ context.Context, got contractsv1.Envelope) error {
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

func TestReadLiveLineBoundsUnterminatedInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(strings.Repeat("x", maxLiveSocketLineBytes+1)))
	line, err := readLiveLine(reader)
	if err == nil || !errors.Is(err, errLiveSocketLineTooLarge) {
		t.Fatalf("read oversized live line error = %v", err)
	}
	if got, want := len(line), maxLiveSocketLineBytes; got != want {
		t.Fatalf("oversized live line prefix length = %d, want %d", got, want)
	}
}

func TestLiveUDSSourceQuarantinesBoundedPrefixForOversizedLine(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	source := NewLiveUDSSource(eventlog.NewEventLog(db), "default", "/tmp/live.sock").
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	prefix := bytes.Repeat([]byte("x"), maxLiveSocketLineBytes)
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	go func() {
		defer client.Close()
		_, _ = client.Write(append(append([]byte(nil), prefix...), 'y'))
	}()
	lines := make(chan liveLine, 1)
	source.readClient(context.Background(), server, 7, lines)
	item := <-lines
	if !errors.Is(item.readErr, errLiveSocketLineTooLarge) {
		t.Fatalf("read error = %v, want oversized-line error", item.readErr)
	}
	if !bytes.Equal(item.data, prefix) {
		t.Fatalf("quarantined prefix differs from bounded input prefix")
	}
	if err := source.processLine(context.Background(), item, func(context.Context, contractsv1.Envelope) error {
		t.Fatal("oversized line reached the sink")
		return nil
	}); err != nil {
		t.Fatalf("process oversized line: %v", err)
	}

	var raw string
	if err := db.QueryRowContext(context.Background(), "SELECT json_extract(payload_json, '$.data.raw') FROM event_quarantine WHERE reason_code = 'line_too_large'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if got, want := raw, string(prefix); got != want {
		t.Fatalf("quarantined raw prefix length/content = %d bytes, want %d", len(got), len(want))
	}
}

func TestLiveUDSSourceAcceptsReconnects(t *testing.T) {
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	path := filepath.Join("/tmp", fmt.Sprintf("agentic-stream-live-%d.sock", time.Now().UnixNano()))
	source := NewLiveUDSSource(eventlog.NewEventLog(db), "default", path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	received := make(chan string, 2)
	runDone := make(chan error, 1)
	go func() {
		runDone <- source.Run(ctx, func(ctx context.Context, env contractsv1.Envelope) error {
			if _, err := source.log.Append(ctx, "default", []contractsv1.Envelope{env}); err != nil {
				return fmt.Errorf("append test event: %w", err)
			}
			received <- env.ID
			return nil
		})
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, statErr := os.Stat(path); statErr == nil {
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
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	path := filepath.Join("/tmp", fmt.Sprintf("agentic-stream-live-deadline-%d.sock", time.Now().UnixNano()))
	source := NewLiveUDSSource(eventlog.NewEventLog(db), "default", path).
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() {
		runDone <- source.Run(ctx, func(context.Context, contractsv1.Envelope) error {
			return context.DeadlineExceeded
		})
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, statErr := os.Stat(path); statErr == nil {
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
