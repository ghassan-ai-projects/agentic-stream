package transport_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/device/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
)

func contractPeer(t *testing.T, conn net.Conn) {
	t.Helper()
	defer func() { _ = conn.Close() }()
	if !writeContractFrame(t, conn, contractstest.ValidFrame("state")) {
		return
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		reply := []string{"receipt", "result"}
		if isStateQuery(line) {
			reply = []string{"state"}
		}
		for _, messageType := range reply {
			if !writeContractFrame(t, conn, contractstest.ValidFrame(messageType)) {
				return
			}
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

func receiveRecord(ctx context.Context, t *testing.T, link *transport.UDS, wantType string) {
	t.Helper()
	frame, err := link.Receive(ctx)
	if err != nil {
		t.Fatalf("receive %s: %v", wantType, err)
	}
	record, err := wire.Decode(frame)
	if err != nil || record["message_type"] != wantType {
		t.Fatalf("want a valid %s, got %v (%v)", wantType, record, err)
	}
}

func TestUDSTransportSpeaksTheDeviceContractOverAUnixSocket(t *testing.T) {
	t.Parallel()
	socket := filepath.Join(workerfake.SocketDir(t), "d.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		contractPeer(t, conn)
	}()

	ctx, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()
	link, err := transport.Dial(ctx, socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = link.Close() }()

	receiveRecord(ctx, t, link, "state")
	command, err := wire.Encode(contractstest.ValidFrame("command"))
	if err != nil {
		t.Fatalf("encode command: %v", err)
	}
	if err := link.Send(ctx, command); err != nil {
		t.Fatalf("send command: %v", err)
	}
	receiveRecord(ctx, t, link, "receipt")
	receiveRecord(ctx, t, link, "result")
	refreshed, err := link.QueryState(ctx)
	if err != nil {
		t.Fatalf("query state: %v", err)
	}
	if record, err := wire.Decode(refreshed); err != nil || record["message_type"] != "state" {
		t.Fatalf("query_state must return a valid state, got %v (%v)", record, err)
	}
}

func TestDialReportsAnUnreachableGateway(t *testing.T) {
	t.Parallel()
	socket := filepath.Join(workerfake.SocketDir(t), "absent.sock")
	link, err := transport.Dial(t.Context(), socket)
	if err == nil || !strings.Contains(err.Error(), "dial device gateway") || link != nil {
		t.Fatalf("dial of an absent socket: link=%v err=%v", link, err)
	}
}

func TestAClosedOrMissingTransportRefusesEveryOperation(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	closed := transport.NewForTest(client)
	if err := closed.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("second close must be a no-op, got %v", err)
	}
	var missing *transport.UDS
	for name, link := range map[string]*transport.UDS{"closed": closed, "missing": missing} {
		want := "device transport is closed"
		if link == nil {
			want = "device transport is not configured"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := link.Send(t.Context(), []byte("{}\n")); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("send error = %v, want %q", err, want)
			}
			if _, err := link.Receive(t.Context()); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("receive error = %v, want %q", err, want)
			}
			if _, err := link.QueryState(t.Context()); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("query state error = %v, want %q", err, want)
			}
		})
	}
	if err := missing.Close(); err != nil {
		t.Fatalf("closing a missing transport must be a no-op, got %v", err)
	}
}
