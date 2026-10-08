package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
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
	listener, err := worker.ListenEvidenceSocket(socketPath)
	if err != nil {
		t.Fatalf("listen worker socket: %v", err)
	}
	server := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(server, &workerfake.Server{WorkerName: "tamoz", WorkerVersion: "stand-in", ExecuteFunc: proposeIndicatorAlertAfter(delay)})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
}

func proposeIndicatorAlertAfter(delay time.Duration) workerfake.ExecuteFunc {
	return func(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		return proposeIndicatorAlert(req, emit)
	}
}

func proposeIndicatorAlert(req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	decision, err := indicatorDecision(req)
	if err != nil {
		return err
	}
	decisionJSON, digest, err := digestedDecision(decision)
	if err != nil {
		return err
	}
	// Like Tamoz's EpisodeStream: the budget update and the terminal usage
	// prove the attempt stayed inside its limits.
	usage := &runtimev1.Usage{InputTokens: 1200, OutputTokens: 300}
	if err := emit(episodeEvent(req, 2, &runtimev1.EpisodeEvent_Budget{Budget: &runtimev1.BudgetUpdated{CumulativeUsage: usage, ModelCallsUsed: 1}})); err != nil {
		return err
	}
	if err := emit(episodeEvent(req, 3, &runtimev1.EpisodeEvent_Decision{Decision: &runtimev1.DecisionProposed{
		DecisionJson: decisionJSON, DecisionSha256: digest, EpisodeId: req.GetEpisodeId(), AttemptId: req.GetAttemptId(), Fence: req.GetFence(),
	}})); err != nil {
		return err
	}
	return emit(episodeEvent(req, 4, &runtimev1.EpisodeEvent_Terminal{Terminal: &runtimev1.Terminal{
		Status: runtimev1.TerminalStatus_TERMINAL_STATUS_PRODUCED, ReasonCode: "decision_produced", Usage: usage,
	}}))
}

func indicatorDecision(req *runtimev1.EpisodeRequest) (map[string]any, error) {
	entityID, err := snapshotEntityID(req.GetSnapshotJson())
	if err != nil {
		return nil, err
	}
	scope := fmt.Sprintf("%s.%s.%d", req.GetEpisodeId(), req.GetAttemptId(), req.GetFence())
	validUntil := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	intent := map[string]any{
		"intent_id": "intent." + scope + ".set_indicator", "decision_id": "decision." + scope,
		"tenant_id": req.GetTenantId(), "situation_id": req.GetSituationId(), "situation_version": req.GetSituationVersion(),
		"type": "set_indicator", "risk_class": "R1", "evidence_ids": []any{}, "expires_at": validUntil,
		"parameters": map[string]any{"entity_id": entityID, "situation_id": req.GetSituationId(), "situation_version": req.GetSituationVersion(), "state": "alert"},
	}
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		return nil, fmt.Errorf("digest intent: %w", err)
	}
	intent["intent_digest"] = intentDigest
	return map[string]any{
		"decision_id": "decision." + scope, "episode_id": req.GetEpisodeId(), "attempt_id": req.GetAttemptId(), "fence": req.GetFence(),
		"snapshot_digest": "sha256:" + hex.EncodeToString(req.GetSnapshotSha256()),
		"situation_id":    req.GetSituationId(), "situation_version": req.GetSituationVersion(),
		"primary_hypothesis": "over_temperature", "confidence": 0.9, "summary": "sustained rise above the band",
		"facts_used": []any{}, "alternatives": []any{}, "intents": []any{intent}, "valid_until": validUntil,
	}, nil
}

func snapshotEntityID(snapshotJSON []byte) (string, error) {
	var snapshot struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil || snapshot.Entity.ID == "" {
		return "", fmt.Errorf("snapshot has no entity id: %w", err)
	}
	return snapshot.Entity.ID, nil
}

func digestedDecision(decision map[string]any) ([]byte, []byte, error) {
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal decision: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return nil, nil, fmt.Errorf("digest decision: %w", err)
	}
	digestBytes, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return nil, nil, fmt.Errorf("decode decision digest: %w", err)
	}
	return decisionJSON, digestBytes, nil
}

func episodeEvent(req *runtimev1.EpisodeRequest, sequence uint64, payload any) *runtimev1.EpisodeEvent {
	event := &runtimev1.EpisodeEvent{EpisodeId: req.GetEpisodeId(), Sequence: sequence, AttemptId: req.GetAttemptId(), Fence: req.GetFence(), OccurredAt: timestamppb.Now()}
	switch p := payload.(type) {
	case *runtimev1.EpisodeEvent_Budget:
		event.Payload = p
	case *runtimev1.EpisodeEvent_Decision:
		event.Payload = p
	case *runtimev1.EpisodeEvent_Terminal:
		event.Payload = p
	}
	return event
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
