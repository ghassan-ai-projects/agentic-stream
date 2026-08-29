package actions_test

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	write := func(doc map[string]any) bool {
		frame, err := canonicaljson.Marshal(doc)
		if err != nil {
			t.Errorf("peer marshal: %v", err)
			return false
		}
		_, err = conn.Write(append(frame, '\n'))
		return err == nil
	}
	if !write(contractsv1.ConformanceValidFrame("state")) {
		return
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.Contains(line, `"query_state"`) {
			if !write(contractsv1.ConformanceValidFrame("state")) {
				return
			}
			continue
		}
		if !write(contractsv1.ConformanceValidFrame("receipt")) {
			return
		}
	}
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
