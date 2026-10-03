package ingress

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

func (s *LiveUDSSource) acceptClients(ctx context.Context, listener net.Listener, lines chan<- liveLine, done chan<- struct{}, acceptErr chan<- error, clients *sync.WaitGroup) {
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

// reportAcceptFailure reports an accept error unless the loop is shutting
// down.
func reportAcceptFailure(ctx context.Context, err error, acceptErr chan<- error) {
	if ctx.Err() == nil {
		acceptErr <- fmt.Errorf("accept live ingress client: %w", err)
	}
}

// admitClient starts reading the connection, or closes it when the client
// limit is reached.
func (s *LiveUDSSource) admitClient(ctx context.Context, conn net.Conn, lines chan<- liveLine, clients *sync.WaitGroup) {
	if s.clientCount.Load() >= maxLiveSocketClients {
		s.logger.WarnContext(ctx, "live ingress client rejected", "source", liveSocketSourceTag, "reason_code", "client_limit")
		_ = conn.Close()
		return
	}
	s.startClient(ctx, conn, lines, clients)
}

// retryAccept reports whether a failed accept should be retried: a timeout
// waits briefly and retries, while shutdown and other errors stop accepting.
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
func (s *LiveUDSSource) startClient(ctx context.Context, conn net.Conn, lines chan<- liveLine, clients *sync.WaitGroup) {
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

func (s *LiveUDSSource) readClient(ctx context.Context, conn net.Conn, connectionID uint64, lines chan<- liveLine) {
	reader, lineNumber := bufio.NewReaderSize(conn, maxLiveSocketLineBytes), uint64(0)
	for {
		line, err := readLiveLine(reader)
		if errors.Is(err, io.EOF) {
			return
		}
		lineNumber++
		if err == nil && len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		item := liveLine{connectionID: connectionID, lineNumber: lineNumber, data: line, readErr: err}
		if !enqueueLine(ctx, lines, item) || err != nil {
			return
		}
	}
}

// enqueueLine hands the line to the processor, reporting false when the
// context ends first.
func enqueueLine(ctx context.Context, lines chan<- liveLine, item liveLine) bool {
	select {
	case lines <- item:
		return true
	case <-ctx.Done():
		return false
	}
}
