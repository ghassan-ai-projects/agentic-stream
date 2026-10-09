package transport

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
)

const waitLimit = 5 * time.Second

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type runningServer struct {
	path   string
	cancel context.CancelFunc
	done   <-chan error
}

func startServer(t *testing.T, queueSize int, handle LineHandler) runningServer {
	t.Helper()
	path := socketPath(t)
	listener, err := listenSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	s := &server{logger: quiet(), connections: make(map[net.Conn]struct{})}
	go func() { done <- s.serve(ctx, listener, queueSize, handle) }()
	t.Cleanup(cancel)
	return runningServer{path: path, cancel: cancel, done: done}
}

func (r runningServer) dial(t *testing.T) net.Conn {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", r.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func writeLine(t *testing.T, conn net.Conn, text string) {
	t.Helper()
	if _, err := conn.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
}

func receive[T any](t *testing.T, from <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-from:
		return value
	case <-time.After(waitLimit):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func TestServeDeliversLinesInOrderAndShutsDownCleanly(t *testing.T) {
	t.Parallel()
	got := make(chan domain.LiveLine, 4)
	running := startServer(t, 4, func(_ context.Context, line domain.LiveLine) error {
		got <- line
		return nil
	})
	conn := running.dial(t)
	writeLine(t, conn, "first\n\nsecond\n")

	for _, want := range []struct {
		data       string
		lineNumber uint64
	}{{"first\n", 1}, {"second\n", 3}} {
		line := receive(t, got, "a line")
		if string(line.Data) != want.data || line.LineNumber != want.lineNumber || line.ReadErr != nil {
			t.Fatalf("line = %+v, want %q as line %d (blank lines count)", line, want.data, want.lineNumber)
		}
	}
	running.cancel()
	if err := receive(t, running.done, "shutdown"); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestServeNumbersEachConnectionSeparately(t *testing.T) {
	t.Parallel()
	got := make(chan domain.LiveLine, 2)
	running := startServer(t, 2, func(_ context.Context, line domain.LiveLine) error {
		got <- line
		return nil
	})

	writeLine(t, running.dial(t), "a\n")
	first := receive(t, got, "the first connection's line")
	writeLine(t, running.dial(t), "b\n")
	second := receive(t, got, "the second connection's line")

	if first.ConnectionID == second.ConnectionID || first.LineNumber != 1 || second.LineNumber != 1 {
		t.Fatalf("first = %+v, second = %+v, want distinct connections that each restart at line 1", first, second)
	}
}

func TestServeStopsWithTheHandlerError(t *testing.T) {
	t.Parallel()
	boom := errors.New("sink failed")
	running := startServer(t, 1, func(context.Context, domain.LiveLine) error { return boom })
	writeLine(t, running.dial(t), "x\n")

	err := receive(t, running.done, "the server to stop")

	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "process live ingress line") {
		t.Fatalf("err = %v, want the handler error wrapped by process live ingress line", err)
	}
}

func TestServeRefusesAnUnsafeSocketPath(t *testing.T) {
	t.Parallel()
	existing := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	err := Serve(t.Context(), ServerConfig{Path: existing, QueueSize: 1, Logger: quiet()}, nil)

	if err == nil || !strings.Contains(err.Error(), "listen live ingress socket") || !strings.Contains(err.Error(), "unsafe existing") {
		t.Fatalf("err = %v, want the listen failure naming the unsafe path", err)
	}
}

func TestClientLimitClosesTheSeventeenthConnection(t *testing.T) {
	t.Parallel()
	running := startServer(t, 1, func(context.Context, domain.LiveLine) error { return nil })
	for range domain.MaxLiveClients {
		running.dial(t)
	}

	extra := running.dial(t)
	if err := extra.SetReadDeadline(time.Now().Add(waitLimit)); err != nil {
		t.Fatal(err)
	}
	_, err := extra.Read(make([]byte, 1))

	if !errors.Is(err, io.EOF) {
		t.Fatalf("read on a client over the limit = %v, want the server to close it (EOF)", err)
	}
}

func TestOversizedClientFrameKeepsOnlyTheBoundedPrefix(t *testing.T) {
	t.Parallel()
	s := &server{logger: quiet(), connections: make(map[net.Conn]struct{})}
	prefix := bytes.Repeat([]byte("x"), domain.MaxLineBytes)
	serverEnd, clientEnd := net.Pipe()
	defer func() { _ = serverEnd.Close() }()
	go func() {
		defer func() { _ = clientEnd.Close() }()
		_, _ = clientEnd.Write(append(append([]byte(nil), prefix...), 'y'))
	}()
	lines := make(chan domain.LiveLine, 1)

	s.readClient(t.Context(), serverEnd, 7, lines)

	item := <-lines
	if !errors.Is(item.ReadErr, domain.ErrLineTooLarge) || !bytes.Equal(item.Data, prefix) || item.ConnectionID != 7 {
		t.Fatalf("item = connection %d, %d bytes, read error %v; want the bounded prefix and an oversized-line error", item.ConnectionID, len(item.Data), item.ReadErr)
	}
}

func TestReadLiveLineBoundsUnterminatedInput(t *testing.T) {
	t.Parallel()
	reader := bufio.NewReader(strings.NewReader(strings.Repeat("x", domain.MaxLineBytes+1)))

	line, err := readLiveLine(reader)

	if !errors.Is(err, domain.ErrLineTooLarge) || len(line) != domain.MaxLineBytes {
		t.Fatalf("prefix length = %d, err = %v, want %d bytes and the oversized-line error", len(line), err, domain.MaxLineBytes)
	}
}

func TestReadLiveLineAcceptsAFinalLineWithoutANewline(t *testing.T) {
	t.Parallel()
	reader := bufio.NewReader(strings.NewReader("tail"))

	line, err := readLiveLine(reader)
	if err != nil || string(line) != "tail" {
		t.Fatalf("line = %q, err = %v, want the unterminated final line", line, err)
	}
	if _, err := readLiveLine(reader); !errors.Is(err, io.EOF) {
		t.Fatalf("after the last line: err = %v, want EOF", err)
	}
}

func TestShutdownIsNormalOnlyWhileTheContextIsDone(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	boom := errors.New("boom")
	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		wantErr bool
	}{
		{name: "canceled during shutdown", ctx: canceled, err: context.Canceled},
		{name: "deadline during shutdown", ctx: canceled, err: context.DeadlineExceeded},
		{name: "other failure during shutdown", ctx: canceled, err: boom, wantErr: true},
		{name: "deadline while running", ctx: t.Context(), err: context.DeadlineExceeded, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := lineFailure(tt.ctx, tt.err)
			if (err != nil) != tt.wantErr || (tt.wantErr && !errors.Is(err, tt.err)) {
				t.Fatalf("lineFailure = %v, want an error: %t that wraps %v", err, tt.wantErr, tt.err)
			}
		})
	}
}

type scriptedListener struct {
	net.Listener
	accepts chan error
}

func (l scriptedListener) Accept() (net.Conn, error) { return nil, <-l.accepts }
func (l scriptedListener) Close() error              { return nil }

type timeoutError struct{}

func (timeoutError) Error() string   { return "accept timed out" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestServeRetriesATimedOutAcceptAndStopsOnAnyOtherAcceptFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("listener broke")
	accepts := make(chan error, 3)
	accepts <- timeoutError{}
	accepts <- timeoutError{}
	accepts <- boom
	s := &server{logger: quiet(), connections: make(map[net.Conn]struct{})}

	err := s.serve(t.Context(), scriptedListener{accepts: accepts}, 1, func(context.Context, domain.LiveLine) error { return nil })

	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "accept live ingress client") || len(accepts) != 0 {
		t.Fatalf("err = %v with %d scripted accepts left, want the third accept's failure after two retries", err, len(accepts))
	}
}
