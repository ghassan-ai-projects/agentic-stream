package actions

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestUDSTransportMarksPartialWriteAsPossiblySent(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	cause := errors.New("write interrupted")
	transport := newUDSTransport(&partialWriteConn{Conn: client, cause: cause})

	err := transport.Send(context.Background(), []byte("{}\n"))
	if !errors.Is(err, cause) {
		t.Fatalf("send error = %v, want %v", err, cause)
	}
	if !transportMayHaveSent(err) {
		t.Fatal("partial write was not classified as possibly sent")
	}
}

func TestPrepareDeadlineClosesTransportWhenCancellationDeadlineFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
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
	case <-time.After(time.Second):
		t.Fatal("transport was not closed after cancellation deadline failure")
	}
	if err := cleanup(); !errors.Is(err, cause) {
		t.Fatalf("cleanup error = %v, want %v", err, cause)
	}
}

func TestUDSQueryStateDoesNotHoldReadLockWhileWaitingToWrite(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	transport := newUDSTransport(client)
	<-transport.writeGate
	transport.readMu.Lock()

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &observedContext{Context: baseCtx, observed: make(chan struct{})}
	queryDone := make(chan error, 1)
	go func() {
		_, err := transport.QueryState(ctx)
		queryDone <- err
	}()
	select {
	case <-ctx.observed:
	case <-time.After(time.Second):
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
	case <-time.After(time.Second):
		t.Fatal("query state held read lock while waiting for write gate")
	}
	cancel()
	select {
	case <-queryDone:
	case <-time.After(time.Second):
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
