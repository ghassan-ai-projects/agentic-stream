package transport

import (
	"context"
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

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
)

func socketPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("/tmp", fmt.Sprintf("agentic-ingress-%d.sock", time.Now().UnixNano()))
}

func skipWithoutSockets(t *testing.T, err error) {
	t.Helper()
	if err != nil && (errors.Is(err, syscall.EPERM) || strings.Contains(err.Error(), "operation not permitted")) {
		t.Skipf("Unix socket listeners unavailable: %v", err)
	}
}

func waitForSocket(t *testing.T, path string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case err := <-done:
			skipWithoutSockets(t, err)
			t.Fatalf("server stopped before listening: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not create its socket")
		}
		time.Sleep(time.Millisecond)
	}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestServeDeliversLinesInOrderAndShutsDownCleanly(t *testing.T) {
	path := socketPath(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	got := make(chan string, 4)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ServerConfig{Path: path, QueueSize: 4, Logger: quiet()}, func(_ context.Context, line domain.LiveLine) error {
			got <- string(line.Data)
			return nil
		})
	}()
	waitForSocket(t, path, done)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v err=%v, want owner-only", info.Mode().Perm(), err)
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("first\n\nsecond\n"))
	_ = conn.Close()
	for _, want := range []string{"first\n", "second\n"} {
		select {
		case line := <-got:
			if line != want {
				t.Fatalf("line = %q, want %q", line, want)
			}
		case <-time.After(time.Second):
			t.Fatal("line not delivered")
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the socket file must be removed on shutdown: %v", err)
	}
}

func TestServeStopsWithTheHandlerError(t *testing.T) {
	path := socketPath(t)
	boom := errors.New("sink failed")
	done := make(chan error, 1)
	go func() {
		done <- Serve(t.Context(), ServerConfig{Path: path, QueueSize: 1, Logger: quiet()}, func(context.Context, domain.LiveLine) error { return boom })
	}()
	waitForSocket(t, path, done)
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("x\n"))
	_ = conn.Close()
	select {
	case err := <-done:
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the handler error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop on the handler error")
	}
}

func TestListenerRefusesUnsafeExistingPaths(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "file")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"regular file": regular, "symlink": link} {
		if _, err := listenSocket(path); err == nil || !strings.Contains(err.Error(), "unsafe existing") {
			t.Fatalf("%s: err = %v, want an unsafe-path refusal", name, err)
		}
	}
}

func TestListenerRefusesAnActiveSocket(t *testing.T) {
	path := socketPath(t)
	first, err := listenSocket(path)
	skipWithoutSockets(t, err)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if _, err := listenSocket(path); err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("err = %v, want an already-active refusal", err)
	}
}

func TestClientLimitClosesTheSeventeenthConnection(t *testing.T) {
	path := socketPath(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ServerConfig{Path: path, QueueSize: 1, Logger: quiet()}, func(context.Context, domain.LiveLine) error { return nil })
	}()
	waitForSocket(t, path, done)
	var held []net.Conn
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()
	for i := 0; i < domain.MaxLiveClients; i++ {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
	}
	time.Sleep(50 * time.Millisecond)
	extra, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = extra.Close() }()
	_ = extra.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := extra.Read(make([]byte, 1)); err == nil {
		t.Fatal("a client over the limit was kept open")
	}
}

func TestEachScannedLineCountsBlankLinesAndStopsOnError(t *testing.T) {
	var lines []string
	if err := EachScannedLine(strings.NewReader("a\n\nb\n"), func(line []byte) error { lines = append(lines, string(line)); return nil }); err != nil || len(lines) != 3 {
		t.Fatalf("lines = %q err=%v", lines, err)
	}
	stop := errors.New("stop")
	if err := EachScannedLine(strings.NewReader("a\nb\n"), func([]byte) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("err = %v", err)
	}
	if err := EachScannedLine(strings.NewReader(strings.Repeat("x", 70*1024)), func([]byte) error { return nil }); err == nil {
		t.Fatal("an over-long scanned line was accepted")
	}
}

func TestOpenTraceNamesTheMissingFile(t *testing.T) {
	if _, err := OpenTrace(filepath.Join(t.TempDir(), "absent"), "trace file"); err == nil || !strings.Contains(err.Error(), "open trace file") {
		t.Fatalf("err = %v", err)
	}
}
