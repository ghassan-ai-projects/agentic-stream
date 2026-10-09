package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const waitLimit = 5 * time.Second

type liveSource struct {
	path   string
	cancel context.CancelFunc
	done   <-chan error
}

func startLiveSource(t *testing.T, source *Service, sink EnvelopeSink) liveSource {
	t.Helper()
	dir, err := os.MkdirTemp("", "as-") //nolint:usetesting // A Unix socket path is limited to about 100 bytes; t.TempDir() paths are longer.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "live.sock")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- source.ServeLive(ctx, path, sink) }()
	running := liveSource{path: path, cancel: cancel, done: done}
	running.waitUntilListening(t)
	return running
}

func (s liveSource) waitUntilListening(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), waitLimit)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if probe, err := (&net.Dialer{}).DialContext(ctx, "unix", s.path); err == nil {
			_ = probe.Close()
			return
		}
		select {
		case err := <-s.done:
			t.Fatalf("live source stopped before listening: %v", err)
		case <-ctx.Done():
			t.Fatal("live source did not start listening")
		case <-ticker.C:
		}
	}
}

func (s liveSource) send(t *testing.T, lines ...[]byte) {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for _, line := range lines {
		if _, err := conn.Write(line); err != nil {
			t.Fatal(err)
		}
	}
}

func (s liveSource) stop(t *testing.T) error {
	t.Helper()
	s.cancel()
	return s.await(t)
}

func (s liveSource) await(t *testing.T) error {
	t.Helper()
	select {
	case err := <-s.done:
		return err
	case <-time.After(waitLimit):
		t.Fatal("live source did not stop")
		return nil
	}
}

func expectDelivery(t *testing.T, received <-chan string, want string) {
	t.Helper()
	select {
	case got := <-received:
		if got != want {
			t.Fatalf("delivered %q, want %q", got, want)
		}
	case <-time.After(waitLimit):
		t.Fatalf("live source did not deliver %s", want)
	}
}

func TestServeLiveAcceptsReconnectingClientsAndQuarantinesMalformedInput(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	source := newLiveService(t, db, nil)
	received := make(chan string, 2)
	running := startLiveSource(t, source, func(_ context.Context, env contractsv1.Envelope) error {
		received <- env.ID
		return nil
	})

	running.send(t, []byte("not-json\n"), liveEnvelopeLine(t, "evt-live-1"))
	expectDelivery(t, received, "evt-live-1")
	running.send(t, liveEnvelopeLine(t, "evt-live-2"))
	expectDelivery(t, received, "evt-live-2")

	if err := running.stop(t); err != nil {
		t.Fatalf("a canceled context is a normal shutdown, got %v", err)
	}
	if raw := quarantinedRaw(t, db, "malformed_json"); raw != "not-json\n" {
		t.Fatalf("quarantined raw line = %q, want the malformed line", raw)
	}
}

func TestServeLiveReportsASinkDeadlineWhileTheContextIsActive(t *testing.T) {
	t.Parallel()
	source := newLiveService(t, storagetest.OpenTemp(t), nil)
	running := startLiveSource(t, source, func(context.Context, contractsv1.Envelope) error {
		return context.DeadlineExceeded
	})

	running.send(t, liveEnvelopeLine(t, "evt-live-deadline"))

	if err := running.await(t); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sink deadline result = %v, want the propagated deadline", err)
	}
}

func TestServeLiveRefusesWithoutASinkOrWithAnUnsafePath(t *testing.T) {
	t.Parallel()
	source := newLiveService(t, storagetest.OpenTemp(t), nil)
	sink := func(context.Context, contractsv1.Envelope) error { return nil }
	tests := []struct {
		name string
		path string
		sink EnvelopeSink
		want string
	}{
		{name: "no sink", path: "/tmp/live.sock", sink: nil, want: "sink is required"},
		{name: "relative path", path: "live.sock", sink: sink, want: "clean absolute Unix path"},
		{name: "path that is not clean", path: "/tmp/../live.sock", sink: sink, want: "clean absolute Unix path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := source.ServeLive(t.Context(), tt.path, tt.sink); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
