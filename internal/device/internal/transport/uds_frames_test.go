package transport_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
)

const waitBound = 5 * time.Second

func pipeTransport(t *testing.T) (*transport.UDS, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	return transport.NewForTest(client), server
}

func oversizedFrameWriter(server net.Conn) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = server.Write(bytes.Repeat([]byte{'x'}, wire.MaxFrameBytes))
		_, _ = server.Write([]byte{'x'})
		_ = server.Close()
	}()
	return done
}

func waitFor(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(waitBound):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestUDSTransportRejectsAnOversizedFrameAndClosesTheLink(t *testing.T) {
	t.Parallel()
	link, server := pipeTransport(t)
	writerDone := oversizedFrameWriter(server)

	if _, err := link.Receive(t.Context()); err == nil || !strings.Contains(err.Error(), "exceeds 65536 bytes") {
		t.Fatalf("oversized frame error = %v, want a bounded-frame error", err)
	}
	if _, err := link.Receive(t.Context()); err == nil || !strings.Contains(err.Error(), "device transport is closed") {
		t.Fatalf("transport remained usable after an oversized frame: %v", err)
	}
	waitFor(t, writerDone, "the oversized frame writer to exit")
}

func TestUDSTransportOperationsHonorCancellation(t *testing.T) {
	t.Parallel()
	cases := map[string]func(ctx context.Context, link *transport.UDS) error{
		"receive": func(ctx context.Context, link *transport.UDS) error {
			_, err := link.Receive(ctx)
			return err
		},
		"send": func(ctx context.Context, link *transport.UDS) error {
			return link.Send(ctx, append(bytes.Repeat([]byte{'x'}, wire.MaxFrameBytes-1), '\n'))
		},
	}
	for name, operate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, server := net.Pipe()
			defer func() { _ = client.Close(); _ = server.Close() }()
			conn := newNotifyingConn(client)
			link := transport.NewForTest(conn)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			errCh := make(chan error, 1)
			go func() { errCh <- operate(ctx, link) }()
			waitFor(t, conn.started(name), name+" to reach the connection")
			cancel()
			select {
			case err := <-errCh:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("%s error = %v, want context.Canceled", name, err)
				}
			case <-time.After(waitBound):
				t.Fatalf("%s did not honor cancellation", name)
			}
		})
	}
}

func TestUDSTransportSendWaitHonorsCancellation(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	conn := newNotifyingConn(client)
	link := transport.NewForTest(conn)
	firstErr := make(chan error, 1)
	go func() {
		firstErr <- link.Send(t.Context(), append(bytes.Repeat([]byte{'x'}, wire.MaxFrameBytes-1), '\n'))
	}()
	waitFor(t, conn.writeStarted.ch, "the first send to block on the connection")

	ctx, cancel := context.WithCancel(t.Context())
	secondErr := make(chan error, 1)
	go func() { secondErr <- link.Send(ctx, []byte("{}\n")) }()
	cancel()
	select {
	case err := <-secondErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting send error = %v, want context.Canceled", err)
		}
	case <-time.After(waitBound):
		t.Fatal("a send waiting for the write gate did not honor cancellation")
	}

	_ = client.Close()
	if err := <-firstErr; err == nil {
		t.Fatal("the blocked first send unexpectedly succeeded")
	}
}

func TestUDSTransportSendValidatesOneBoundedFrame(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		frame []byte
		want  string
	}{
		"empty":           {nil, "device frame is empty"},
		"missing newline": {[]byte(`{"message_type":"command"}`), "exactly one trailing newline"},
		"multiple lines":  {[]byte("{}\n{}\n"), "exactly one trailing newline"},
		"oversized":       {append(bytes.Repeat([]byte{'x'}, wire.MaxFrameBytes), '\n'), "exceeds 65536 bytes"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			link, _ := pipeTransport(t)
			err := link.Send(t.Context(), tc.frame)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("send error = %v, want %q", err, tc.want)
			}
			if transport.MayHaveSent(err) {
				t.Fatalf("a frame refused by validation was classified as possibly sent: %v", err)
			}
		})
	}
}

type signal struct {
	once sync.Once
	ch   chan struct{}
}

func newSignal() *signal { return &signal{ch: make(chan struct{})} }

func (s *signal) fire() { s.once.Do(func() { close(s.ch) }) }

type notifyingConn struct {
	net.Conn
	readStarted  *signal
	writeStarted *signal
}

func newNotifyingConn(conn net.Conn) *notifyingConn {
	return &notifyingConn{Conn: conn, readStarted: newSignal(), writeStarted: newSignal()}
}

func (c *notifyingConn) started(operation string) <-chan struct{} {
	if operation == "receive" {
		return c.readStarted.ch
	}
	return c.writeStarted.ch
}

func (c *notifyingConn) Read(p []byte) (int, error) {
	c.readStarted.fire()
	count, err := c.Conn.Read(p)
	return count, wrapConnError("read", err)
}

func (c *notifyingConn) Write(p []byte) (int, error) {
	c.writeStarted.fire()
	count, err := c.Conn.Write(p)
	return count, wrapConnError("write", err)
}

func wrapConnError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s notifying connection: %w", operation, err)
}
