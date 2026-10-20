package plantwirefake_test

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func TestNodeSequencesWhatItSendsAndReadsTheHostsRecords(t *testing.T) {
	t.Parallel()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	node := plantwirefake.Dial(t, "tcp", listener.Addr().String(), "s-9")
	host, err := listener.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	node.Send(plantwirefake.Fixture(t, ".", "NSYNC.json"))
	node.Skip()
	node.Send(map[string]any{"type": "NDEATH", "seq": 99})
	node.SendLine([]byte(`{"raw":true}`))
	lines := bufio.NewScanner(host)
	for _, want := range []string{`"seq":0`, `"seq":2`, `"raw":true`} {
		if !lines.Scan() || !json.Valid(lines.Bytes()) || !strings.Contains(lines.Text(), want) || want != `"raw":true` && !strings.Contains(lines.Text(), `"session_id":"s-9"`) {
			t.Fatalf("the host read %q, want %s in session s-9", lines.Text(), want)
		}
	}
	if _, err := host.Write([]byte("{\"type\":\"STATE\",\"heartbeat\":3}\n")); err != nil {
		t.Fatal(err)
	}
	if state := node.Expect("STATE"); state["heartbeat"] != json.Number("3") {
		t.Fatalf("STATE = %v, want heartbeat 3 as an exact number", state)
	}
	_ = host.Close()
	if !node.Closed() {
		t.Fatal("Closed = false after the host closed")
	}
	node.Close()
}

func TestKindsDirHoldsTheContractKindFile(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(plantwirefake.KindsDir(t, "."), "pump.kind.json"))
	if err != nil || !json.Valid(raw) {
		t.Fatalf("pump.kind.json = %s, %v", raw, err)
	}
	if birth := plantwirefake.Fixture(t, ".", "NBIRTH.json"); birth["type"] != "NBIRTH" {
		t.Fatalf("the NBIRTH fixture reads as %v", birth["type"])
	}
}
