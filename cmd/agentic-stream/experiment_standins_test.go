package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// The experiment's other two processes, reduced to what this repository can
// observe at its boundaries: a Tamoz-shaped worker on the gRPC protocol and a
// Streams-Simulator-shaped device on the device wire. Both speak only the
// public contracts.

// standInFirmwareDigest is the firmware the stand-in device reports.
const standInFirmwareDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

// serveTamozStandIn serves an EpisodeWorker that answers like Tamoz's
// DecisionBuilder: one actionable intent from the request's catalog, with the
// model writing only the catalog's model-writable field. It reasons for delay
// before deciding, as a model call does.
func serveTamozStandIn(t *testing.T, socketPath string, delay time.Duration) {
	t.Helper()
	serveWorkerProposing(t, socketPath, delay, indicatorAlert)
}

// proposal is one intent the stand-in worker proposes: its type, risk class
// and the model-writable parameters.
type proposal struct {
	intentType, risk string
	parameters       map[string]any
}

var indicatorAlert = proposal{intentType: "set_indicator", risk: "R1", parameters: map[string]any{"state": "alert"}}

func serveWorkerProposing(t *testing.T, socketPath string, delay time.Duration, proposals ...proposal) {
	t.Helper()
	listener, err := worker.ListenEvidenceSocket(socketPath)
	if err != nil {
		t.Fatalf("listen worker socket: %v", err)
	}
	server := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(server, &workerfake.Server{WorkerName: "tamoz", WorkerVersion: "stand-in", ExecuteFunc: proposeAfter(delay, proposals)})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
}

// proposeAfter proposes one intent per episode, as a decision carries at most
// one actionable intent: the proposals in order, then the last one again.
func proposeAfter(delay time.Duration, proposals []proposal) workerfake.ExecuteFunc {
	var episodes atomic.Int64
	return func(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		next := min(int(episodes.Add(1))-1, len(proposals)-1)
		return propose(req, emit, proposals[next])
	}
}

func propose(req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error, proposed proposal) error {
	return workerfake.Propose(req, emit, workerfake.Answer{Hypothesis: "over_temperature", Summary: "sustained rise above the band",
		Intent: &workerfake.ProposedIntent{Type: proposed.intentType, Risk: proposed.risk, Parameters: proposed.parameters}})
}

// deviceStandIn is a device on the wire contract that remembers its output, so
// query_state reports what the last command did, as the Streams Simulator
// emulator and the bench firmware do.
type deviceStandIn struct {
	capabilityDigest string
	mu               sync.Mutex
	output           map[string]any
	commands         []map[string]any
}

func serveDeviceStandIn(t *testing.T, socketPath, capabilityDigest string) *deviceStandIn {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socketPath)
	if err != nil {
		t.Fatalf("listen device socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	device := &deviceStandIn{capabilityDigest: capabilityDigest}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go device.serve(conn)
		}
	}()
	return device
}

func (d *deviceStandIn) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if d.write(conn, d.state()) != nil {
		return
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		if d.answer(conn, line) != nil {
			return
		}
	}
}

func (d *deviceStandIn) answer(conn net.Conn, line []byte) error {
	var frame map[string]any
	if err := json.Unmarshal(line, &frame); err != nil {
		return fmt.Errorf("decode device frame: %w", err)
	}
	if frame["message_type"] == "query_state" {
		return d.write(conn, d.state())
	}
	status := d.apply(frame)
	if err := d.write(conn, map[string]any{"message_type": "receipt", "protocol_version": 1, "command_id": frame["command_id"], "boot_id": "boot-A", "accepted": true, "received_mono_us": 1}); err != nil {
		return err
	}
	return d.write(conn, map[string]any{"message_type": "result", "protocol_version": 1, "command_id": frame["command_id"], "boot_id": "boot-A", "status": status, "completed_mono_us": 2})
}

// apply records the command and sets the output to its single numeric
// parameter, the way the firmware drives an LED level or a fan duty.
func (d *deviceStandIn) apply(command map[string]any) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.commands = append(d.commands, command)
	if command["operation"] == "safe_stop" {
		d.output = nil
		return "safe_state"
	}
	value := 0.0
	if parameters, ok := command["parameters"].(map[string]any); ok {
		for name, parameter := range parameters {
			if number, isNumber := parameter.(float64); isNumber && name != "lease_ms" {
				value = number
			}
		}
	}
	d.output = map[string]any{"target": command["target"], "operation": command["operation"], "value": value, "energized": value > 0}
	return "executed"
}

func (d *deviceStandIn) state() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := map[string]any{
		"message_type": "state", "protocol_version": 1, "device_id": "thermal-01", "boot_id": "boot-A",
		"firmware_digest": standInFirmwareDigest, "capability_digest": d.capabilityDigest, "safe_state": d.output == nil,
	}
	if d.output != nil {
		state["current_output"] = d.output
	}
	return state
}

func (d *deviceStandIn) received() []map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]map[string]any(nil), d.commands...)
}

func (d *deviceStandIn) write(conn net.Conn, frame map[string]any) error {
	data, err := canonicaljson.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal device frame: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write device frame: %w", err)
	}
	return nil
}
