package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
)

// ServerConfig describes one live socket: where it listens, how many framed
// lines may queue ahead of the handler, and where it logs.
type ServerConfig struct {
	Path      string
	QueueSize int
	Logger    *slog.Logger
}

// LineHandler processes one framed live line. An error stops the server.
type LineHandler func(context.Context, domain.LiveLine) error

// server tracks the clients of one listening socket.
type server struct {
	logger        *slog.Logger
	clientCount   atomic.Int64
	connection    atomic.Uint64
	connectionsMu sync.Mutex
	connections   map[net.Conn]struct{}
}

// Serve listens on cfg.Path and delivers client lines to handle in arrival order
// until ctx ends, handle fails, or accepting fails. It accepts reconnecting
// clients with bounded backpressure and never treats a client disconnect as a
// failure. A canceled context is a normal shutdown and returns nil; shutdown
// closes the listener and every client before returning.
func Serve(ctx context.Context, cfg ServerConfig, handle LineHandler) error {
	listener, err := listenSocket(cfg.Path)
	if err != nil {
		return fmt.Errorf("listen live ingress socket: %w", err)
	}
	defer func() { _ = listener.Close() }()
	s := &server{logger: cfg.Logger, connections: make(map[net.Conn]struct{})}
	return s.serve(ctx, listener, cfg.QueueSize, handle)
}

// serve accepts clients and processes their lines until the end conditions.
func (s *server) serve(ctx context.Context, listener net.Listener, queueSize int, handle LineHandler) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan domain.LiveLine, queueSize)
	acceptDone := make(chan struct{})
	acceptErr := make(chan error, 1)
	var clients sync.WaitGroup
	go s.acceptClients(runCtx, listener, lines, acceptDone, acceptErr, &clients)
	defer func() {
		cancel()
		_ = listener.Close()
		s.closeClients()
		clients.Wait()
		<-acceptDone
	}()
	return s.processLines(ctx, runCtx, lines, acceptDone, acceptErr, handle)
}

// processLines handles queued lines in arrival order until the accept loop
// ends or the caller's context is done; normal shutdown is not an error.
func (s *server) processLines(ctx, runCtx context.Context, lines <-chan domain.LiveLine, acceptDone <-chan struct{}, acceptErr <-chan error, handle LineHandler) error {
	for {
		select {
		case item := <-lines:
			if err := handle(runCtx, item); err != nil {
				return lineFailure(ctx, err)
			}
		case <-acceptDone:
			return firstError(acceptErr)
		case <-ctx.Done():
			return nil
		}
	}
}

func lineFailure(ctx context.Context, err error) error {
	if normalShutdown(ctx, err) {
		return nil
	}
	return fmt.Errorf("process live ingress line: %w", err)
}

// firstError returns a pending error, or nil when none was sent.
func firstError(errs <-chan error) error {
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func normalShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

func (s *server) acceptClients(ctx context.Context, listener net.Listener, lines chan<- domain.LiveLine, done chan<- struct{}, acceptErr chan<- error, clients *sync.WaitGroup) {
	defer close(done)
	for {
		conn, err := listener.Accept()
		if err != nil && retryAccept(ctx, err) {
			continue
		}
		if err != nil {
			reportAcceptFailure(ctx, err, acceptErr)
			return
		}
		s.admitClient(ctx, conn, lines, clients)
	}
}

// reportAcceptFailure reports an accept error unless the loop is shutting down.
func reportAcceptFailure(ctx context.Context, err error, acceptErr chan<- error) {
	if ctx.Err() == nil {
		acceptErr <- fmt.Errorf("accept live ingress client: %w", err)
	}
}

// admitClient starts reading the connection, or closes it when the client limit
// is reached.
func (s *server) admitClient(ctx context.Context, conn net.Conn, lines chan<- domain.LiveLine, clients *sync.WaitGroup) {
	if s.clientCount.Load() >= domain.MaxLiveClients {
		s.logger.WarnContext(ctx, "live ingress client rejected", "source", domain.LiveSourceTag, "reason_code", "client_limit")
		_ = conn.Close()
		return
	}
	s.startClient(ctx, conn, lines, clients)
}

// retryAccept reports whether a failed accept should be retried: a timeout waits
// briefly and retries, while shutdown and other errors stop accepting.
func retryAccept(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var networkErr net.Error
	if !errors.As(err, &networkErr) || !networkErr.Timeout() {
		return false
	}
	timer := time.NewTimer(5 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// startClient registers the connection and reads its lines on a goroutine
// tracked by clients.
func (s *server) startClient(ctx context.Context, conn net.Conn, lines chan<- domain.LiveLine, clients *sync.WaitGroup) {
	connectionID := s.connection.Add(1)
	s.clientCount.Add(1)
	s.addClient(conn)
	clients.Add(1)
	go func() {
		defer clients.Done()
		defer s.clientCount.Add(-1)
		defer s.removeClient(conn)
		s.readClient(ctx, conn, connectionID, lines)
	}()
}

func (s *server) readClient(ctx context.Context, conn net.Conn, connectionID uint64, lines chan<- domain.LiveLine) {
	reader, lineNumber := bufio.NewReaderSize(conn, domain.MaxLineBytes), uint64(0)
	for {
		line, err := readLiveLine(reader)
		if errors.Is(err, io.EOF) {
			return
		}
		lineNumber++
		if err == nil && len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		item := domain.LiveLine{ConnectionID: connectionID, LineNumber: lineNumber, Data: line, ReadErr: err}
		if !enqueueLine(ctx, lines, item) || err != nil {
			return
		}
	}
}

// enqueueLine hands the line to the processor, reporting false when the context
// ends first.
func enqueueLine(ctx context.Context, lines chan<- domain.LiveLine, item domain.LiveLine) bool {
	select {
	case lines <- item:
		return true
	case <-ctx.Done():
		return false
	}
}

// readLiveLine frames one line, keeping the bounded prefix of an oversized
// frame for quarantine without retaining or growing memory for the rest.
func readLiveLine(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, min(domain.MaxLineBytes, reader.Size()))
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > domain.MaxLineBytes {
			return line, domain.ErrLineTooLarge
		}
		line = append(line, part...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return completeLiveLine(line, err)
		}
	}
}

// completeLiveLine accepts a line ended by a newline or by end of input.
func completeLiveLine(line []byte, err error) ([]byte, error) {
	if err == nil || (errors.Is(err, io.EOF) && len(line) > 0) {
		return line, nil
	}
	return nil, fmt.Errorf("read live ingress line: %w", err)
}

func (s *server) addClient(conn net.Conn) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	s.connections[conn] = struct{}{}
}

func (s *server) removeClient(conn net.Conn) {
	s.connectionsMu.Lock()
	delete(s.connections, conn)
	s.connectionsMu.Unlock()
	_ = conn.Close()
}

func (s *server) closeClients() {
	s.connectionsMu.Lock()
	clients := make([]net.Conn, 0, len(s.connections))
	for conn := range s.connections {
		clients = append(clients, conn)
	}
	s.connectionsMu.Unlock()
	for _, conn := range clients {
		_ = conn.Close()
	}
}
