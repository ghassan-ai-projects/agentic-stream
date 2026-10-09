package transport

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

const internalWaitBound = 5 * time.Second

func TestUDSTransportMarksPartialWriteAsPossiblySent(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	cause := errors.New("write interrupted")
	transport := newUDS(&partialWriteConn{Conn: client, cause: cause})

	err := transport.Send(t.Context(), []byte("{}\n"))
	if !errors.Is(err, cause) {
		t.Fatalf("send error = %v, want %v", err, cause)
	}
	if !MayHaveSent(err) {
		t.Fatal("partial write was not classified as possibly sent")
	}
}

func TestPrepareDeadlineClosesTransportWhenCancellationDeadlineFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cause := errors.New("deadline unsupported")
	closed := make(chan struct{})
	cleanup, err := prepareDeadline(ctx, func(deadline time.Time) error {
		if deadline.IsZero() {
			return nil
		}
		return cause
	}, func() error {
		close(closed)
		return nil
	})
	if err != nil {
		t.Fatalf("prepare deadline: %v", err)
	}

	cancel()
	select {
	case <-closed:
	case <-time.After(internalWaitBound):
		t.Fatal("transport was not closed after cancellation deadline failure")
	}
	if err := cleanup(); !errors.Is(err, cause) {
		t.Fatalf("cleanup error = %v, want %v", err, cause)
	}
}

func TestUDSQueryStateDoesNotHoldReadLockWhileWaitingToWrite(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	transport := newUDS(client)
	<-transport.writeGate
	transport.readMu.Lock()

	baseCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &observedContext{Context: baseCtx, observed: make(chan struct{})}
	queryDone := make(chan error, 1)
	go func() {
		_, err := transport.QueryState(ctx)
		queryDone <- err
	}()
	select {
	case <-ctx.observed:
	case <-time.After(internalWaitBound):
		t.Fatal("query state did not wait for the write gate")
	}
	transport.readMu.Unlock()

	readLockAvailable := make(chan struct{})
	go func() {
		transport.readMu.Lock()
		close(readLockAvailable)
		transport.readMu.Unlock()
	}()
	select {
	case <-readLockAvailable:
	case <-time.After(internalWaitBound):
		t.Fatal("query state held read lock while waiting for write gate")
	}
	cancel()
	select {
	case <-queryDone:
	case <-time.After(internalWaitBound):
		t.Fatal("query state did not stop after cancellation")
	}
}

type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

type partialWriteConn struct {
	net.Conn
	cause error
}

func (c *partialWriteConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, c.cause
	}
	return 1, c.cause
}

func TestWriteCleanupPreservesOriginalFailureWithoutResetError(t *testing.T) {
	t.Parallel()
	cause := errors.New("original write failure")
	if got := resetWriteDeadline(cause, func() error { return nil }, 1); got != cause { //nolint:errorlint // Exact identity proves cleanup does not introduce a wrapper when no reset fails.
		t.Fatalf("cleanup replaced original failure: %v", got)
	}
	reset := errors.New("deadline reset failure")
	got := resetWriteDeadline(cause, func() error { return reset }, 1)
	if !errors.Is(got, cause) || !errors.Is(got, reset) || !MayHaveSent(got) {
		t.Fatalf("cleanup lost cause or sent classification: %v", got)
	}
}
