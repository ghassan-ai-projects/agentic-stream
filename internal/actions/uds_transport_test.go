package actions_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// contractPeer is a minimal in-process device that speaks the device wire
// contract over a UDS, exactly as the Streams Simulator emulator does: it emits
// state on connect, answers a command with a receipt, and answers a query_state
// control with a fresh state. It exists to prove UDSTransport's framing against
// the real contract without a cross-repo binary.
func contractPeer(t *testing.T, conn net.Conn) {
	t.Helper()
	defer conn.Close()
	if !writeContractFrame(t, conn, contractsv1.ConformanceValidFrame("state")) {
		return
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		if isStateQuery(line) {
			if !writeContractFrame(t, conn, contractsv1.ConformanceValidFrame("state")) {
				return
			}
			continue
		}
		if !writeContractFrame(t, conn, contractsv1.ConformanceValidFrame("receipt")) {
			return
		}
	}
}

func writeContractFrame(t *testing.T, conn net.Conn, document map[string]any) bool {
	t.Helper()
	frame, err := canonicaljson.Marshal(document)
	if err != nil {
		t.Errorf("peer marshal: %v", err)
		return false
	}
	_, err = conn.Write(append(frame, '\n'))
	return err == nil
}

func isStateQuery(line []byte) bool {
	var request struct {
		MessageType string `json:"message_type"`
	}
	return json.Unmarshal(line, &request) == nil && request.MessageType == "query_state"
}

func TestUDSTransportSpeaksTheContract(t *testing.T) {
	// A Unix socket path must fit the platform's short sun_path limit, so a
	// deep t.TempDir() path will not bind on macOS. Use a short /tmp name.
	dir, err := os.MkdirTemp("/tmp", "as-uds")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "d.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		contractPeer(t, conn)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	transport, err := actions.DialUDSTransport(ctx, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	// Opening handshake: the device emits state on connect, read via Receive.
	stateFrame, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("receive opening state: %v", err)
	}
	state, err := actions.DecodeDeviceRecord(stateFrame)
	if err != nil || state["message_type"] != "state" {
		t.Fatalf("opening frame must be a valid state: %v (%v)", state, err)
	}

	// Send a command, read its receipt.
	command, err := actions.EncodeDeviceRecord(contractsv1.ConformanceValidFrame("command"))
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.Send(ctx, command); err != nil {
		t.Fatalf("send command: %v", err)
	}
	receiptFrame, err := transport.Receive(ctx)
	if err != nil {
		t.Fatalf("receive receipt: %v", err)
	}
	receipt, err := actions.DecodeDeviceRecord(receiptFrame)
	if err != nil || receipt["message_type"] != "receipt" {
		t.Fatalf("expected a valid receipt, got %v (%v)", receipt, err)
	}

	// QueryState requests and reads a fresh state.
	queried, err := transport.QueryState(ctx)
	if err != nil {
		t.Fatalf("query state: %v", err)
	}
	refreshed, err := actions.DecodeDeviceRecord(queried)
	if err != nil || refreshed["message_type"] != "state" {
		t.Fatalf("query_state must return a valid state, got %v (%v)", refreshed, err)
	}
}

func TestUDSTransportRejectsOversizedFrameBeforeDecoding(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	transport := actions.NewUDSTransport(client)

	writeDone := make(chan struct{})
	go func() {
		_, _ = server.Write(bytes.Repeat([]byte{'x'}, 64*1024))
		_, _ = server.Write([]byte{'x'})
		_ = server.Close()
		close(writeDone)
	}()

	_, err := transport.Receive(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeds 65536 bytes") {
		t.Fatalf("oversized frame error = %v, want bounded-frame error", err)
	}
	_ = server.Close()
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("oversized frame writer did not exit")
	}
}

func TestUDSTransportClosesAfterOversizedFrame(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	transport := actions.NewUDSTransport(client)
	writeDone := make(chan struct{})
	go func() {
		_, _ = server.Write(bytes.Repeat([]byte{'x'}, 64*1024))
		_, _ = server.Write([]byte{'x'})
		_ = server.Close()
		close(writeDone)
	}()

	if _, err := transport.Receive(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds 65536 bytes") {
		t.Fatalf("oversized frame error = %v, want bounded-frame error", err)
	}
	if _, err := transport.Receive(context.Background()); err == nil || !strings.Contains(err.Error(), "device transport is closed") {
		t.Fatalf("transport remained usable after oversized frame: %v", err)
	}
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("oversized frame writer did not exit after transport closure")
	}
}

func TestUDSTransportOperationsHonorCancellation(t *testing.T) {
	t.Run("receive", func(t *testing.T) {
		client, server := net.Pipe()
		defer func() { _ = client.Close() }()
		defer func() { _ = server.Close() }()
		conn := newNotifyingConn(client)
		transport := actions.NewUDSTransport(conn)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errCh := make(chan error, 1)
		go func() {
			_, err := transport.Receive(ctx)
			errCh <- err
		}()
		waitForTransportCall(t, conn.readStarted)
		cancel()
		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("receive error = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("receive did not honor cancellation")
		}
	})

	t.Run("send", func(t *testing.T) {
		client, server := net.Pipe()
		defer func() { _ = client.Close() }()
		defer func() { _ = server.Close() }()
		conn := newNotifyingConn(client)
		transport := actions.NewUDSTransport(conn)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errCh := make(chan error, 1)
		frame := append(bytes.Repeat([]byte{'x'}, 64*1024-1), '\n')
		go func() { errCh <- transport.Send(ctx, frame) }()
		waitForTransportCall(t, conn.writeStarted)
		cancel()
		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("send error = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("send did not honor cancellation")
		}
	})
}

func TestUDSTransportSendValidatesOneBoundedFrame(t *testing.T) {
	cases := map[string][]byte{
		"empty":           nil,
		"missing newline": []byte(`{"message_type":"command"}`),
		"multiple lines":  []byte("{}\n{}\n"),
		"oversized":       append(bytes.Repeat([]byte{'x'}, 64*1024), '\n'),
	}
	for name, frame := range cases {
		name, frame := name, frame
		t.Run(name, func(t *testing.T) {
			client, server := net.Pipe()
			defer func() { _ = client.Close() }()
			defer func() { _ = server.Close() }()
			transport := actions.NewUDSTransport(client)
			if err := transport.Send(context.Background(), frame); err == nil {
				t.Fatal("invalid outgoing frame was accepted")
			}
		})
	}
}

func TestUDSTransportSendWaitHonorsCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	conn := newNotifyingConn(client)
	transport := actions.NewUDSTransport(conn)
	firstErr := make(chan error, 1)
	firstFrame := append(bytes.Repeat([]byte{'x'}, 64*1024-1), '\n')
	go func() { firstErr <- transport.Send(context.Background(), firstFrame) }()
	waitForTransportCall(t, conn.writeStarted)

	ctx, cancel := context.WithCancel(context.Background())
	secondErr := make(chan error, 1)
	go func() { secondErr <- transport.Send(ctx, []byte("{}\n")) }()
	cancel()
	select {
	case err := <-secondErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting send error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("send waiting for write gate did not honor cancellation")
	}

	_ = client.Close()
	if err := <-firstErr; err == nil {
		t.Fatal("blocked first send unexpectedly succeeded")
	}
}

type notifyingConn struct {
	net.Conn
	readStarted  chan struct{}
	writeStarted chan struct{}
	readOnce     sync.Once
	writeOnce    sync.Once
}

func newNotifyingConn(conn net.Conn) *notifyingConn {
	return &notifyingConn{
		Conn:         conn,
		readStarted:  make(chan struct{}),
		writeStarted: make(chan struct{}),
	}
}

func (c *notifyingConn) Read(p []byte) (int, error) {
	c.readOnce.Do(func() { close(c.readStarted) })
	count, err := c.Conn.Read(p)
	if err == nil {
		return count, nil
	}
	return count, fmt.Errorf("read notifying connection: %w", err)
}

func (c *notifyingConn) Write(p []byte) (int, error) {
	c.writeOnce.Do(func() { close(c.writeStarted) })
	count, err := c.Conn.Write(p)
	if err == nil {
		return count, nil
	}
	return count, fmt.Errorf("write notifying connection: %w", err)
}

func waitForTransportCall(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("transport operation did not start")
	}
}
