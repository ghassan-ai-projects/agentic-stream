// Package plantwirefake is a scripted plant wire node for tests: it dials a
// host, sends the records a test gives it in sequence within one session, and
// reads the host's records with a deadline. Import it from tests only.
package plantwirefake

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const readTimeout = 5 * time.Second

// Node is one node session dialed to a host.
type Node struct {
	tb      testing.TB
	conn    net.Conn
	reader  *bufio.Reader
	session string
	next    uint64
}

// Dial connects a node session to the host; the connection closes when the
// test ends.
func Dial(tb testing.TB, network, address, session string) *Node {
	tb.Helper()
	conn, err := (&net.Dialer{Timeout: readTimeout}).DialContext(context.Background(), network, address)
	if err != nil {
		tb.Fatalf("dial the plant wire host: %v", err)
	}
	tb.Cleanup(func() { _ = conn.Close() })
	return &Node{tb: tb, conn: conn, reader: bufio.NewReader(conn), session: session}
}

// Send stamps the record with this session, protocol version 1 and the next
// sequence number, and writes it as one line.
func (n *Node) Send(record map[string]any) {
	n.tb.Helper()
	stamped := map[string]any{"session_id": n.session, "protocol_version": 1, "seq": n.next}
	for member, value := range record {
		if _, set := stamped[member]; !set {
			stamped[member] = value
		}
	}
	n.next++
	line, err := json.Marshal(stamped)
	if err != nil {
		n.tb.Fatalf("encode %v: %v", record["type"], err)
	}
	n.SendLine(line)
}

// SendLine writes one raw line, unchanged and unsequenced by the node.
func (n *Node) SendLine(line []byte) {
	n.tb.Helper()
	if _, err := n.conn.Write(append(line, '\n')); err != nil {
		n.tb.Fatalf("write to the host: %v", err)
	}
}

// Skip moves the node's next sequence number forward, leaving a gap.
func (n *Node) Skip() { n.next++ }

// Expect reads the host's next record and fails the test unless it has the
// given type.
func (n *Node) Expect(recordType string) map[string]any {
	n.tb.Helper()
	record, err := n.read()
	if err != nil {
		n.tb.Fatalf("read a %s from the host: %v", recordType, err)
	}
	if record["type"] != recordType {
		n.tb.Fatalf("the host sent %v, want %s", record, recordType)
	}
	return record
}

// Closed reports whether the host closed the session before sending another
// record.
func (n *Node) Closed() bool {
	n.tb.Helper()
	_, err := n.read()
	return err != nil
}

// Close ends the node's side of the session.
func (n *Node) Close() { _ = n.conn.Close() }

func (n *Node) read() (map[string]any, error) {
	if err := n.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return nil, fmt.Errorf("set the read deadline: %w", err)
	}
	line, err := n.reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read a host record: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var record map[string]any
	if err := decoder.Decode(&record); err != nil {
		return nil, fmt.Errorf("decode a host record: %w", err)
	}
	return record, nil
}

// Fixture reads one record of the plant wire contract's valid fixtures, found
// from dir (any directory inside the repository).
func Fixture(tb testing.TB, dir, name string) map[string]any {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join(contractFolder(tb, dir), "fixtures", "valid", name))
	if err != nil {
		tb.Fatalf("read fixture %s: %v", name, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var record map[string]any
	if err := decoder.Decode(&record); err != nil {
		tb.Fatalf("decode fixture %s: %v", name, err)
	}
	return record
}

// KindsDir writes the contract's valid kind file as pump.kind.json into a new
// directory and returns it.
func KindsDir(tb testing.TB, dir string) string {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join(contractFolder(tb, dir), "fixtures", "valid", "KIND.json"))
	if err != nil {
		tb.Fatalf("read the kind fixture: %v", err)
	}
	kinds := tb.TempDir()
	root, err := os.OpenRoot(kinds)
	if err != nil {
		tb.Fatalf("open the kind directory: %v", err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("pump.kind.json", raw, 0o600); err != nil {
		tb.Fatalf("write the kind file: %v", err)
	}
	return kinds
}

func contractFolder(tb testing.TB, dir string) string {
	tb.Helper()
	for current, _ := filepath.Abs(dir); current != filepath.Dir(current); current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return filepath.Join(current, "internal", "plantwire", "internal", "wire", "plantwire-contract")
		}
	}
	tb.Fatalf("no repository root above %s", dir)
	return ""
}
