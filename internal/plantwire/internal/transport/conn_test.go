package transport_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
)

func TestConnReadsOneRecordPerLineAndWritesLines(t *testing.T) {
	t.Parallel()
	conn, peer := accepted(t)
	if _, err := peer.Write([]byte("{\"a\":1}\n{\"b\":2}\n")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"{\"a\":1}\n", "{\"b\":2}\n"} {
		if line, err := conn.ReadLine(); err != nil || string(line) != want {
			t.Fatalf("ReadLine = %q, %v; want %q", line, err, want)
		}
	}
	if written, err := conn.Write([]byte(`{"type":"STATE"}`)); err != nil || written == 0 {
		t.Fatalf("Write = %d, %v", written, err)
	}
	if line, err := bufio.NewReader(peer).ReadString('\n'); err != nil || line != "{\"type\":\"STATE\"}\n" {
		t.Fatalf("the peer read %q, %v", line, err)
	}
}

func TestConnRefusesALineLongerThanTheLargestRecord(t *testing.T) {
	t.Parallel()
	conn, peer := accepted(t)
	go func() { _, _ = peer.Write(bytes.Repeat([]byte("x"), wire.MaxBirthBytes+2)) }()
	if _, err := conn.ReadLine(); !errors.Is(err, transport.ErrLineTooLong) {
		t.Fatalf("ReadLine = %v, want ErrLineTooLong", err)
	}
}

func TestConnReportsTheEndOfTheStreamAndAClosedConnection(t *testing.T) {
	t.Parallel()
	conn, peer := accepted(t)
	_ = peer.Close()
	if _, err := conn.ReadLine(); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadLine after the peer closed = %v, want EOF", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if written, err := conn.Write([]byte("{}")); !errors.Is(err, net.ErrClosed) || written != 0 {
		t.Fatalf("Write after Close = %d, %v, want net.ErrClosed and nothing written", written, err)
	}
	if err := conn.Close(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("a second Close = %v, want net.ErrClosed", err)
	}
}

func TestListenerStopsAcceptingOnceClosed(t *testing.T) {
	t.Parallel()
	if transport.NewListener(nil) != nil {
		t.Fatal("NewListener(nil) built a listener")
	}
	listener := transport.NewListener(tcpListener(t))
	if err := listener.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after Close = %v, want net.ErrClosed", err)
	}
	if err := listener.Close(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("a second Close = %v, want net.ErrClosed", err)
	}
}

func accepted(t *testing.T) (*transport.Conn, net.Conn) {
	t.Helper()
	raw := tcpListener(t)
	listener := transport.NewListener(raw)
	peer, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", raw.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, peer
}

func tcpListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}
