package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// HeartbeatTimer is the processing-time timer that detects a missing heartbeat
// for one operator state key.
type HeartbeatTimer struct {
	ID, OperatorID, StateKey string
	DueAt                    time.Time
	Payload                  []byte
}

// HeartbeatTimers derives the timers to arm for the operators of kind
// missing_heartbeat from the partition's operator state.
func HeartbeatTimers(deploymentID, tenantID string, partitionID int, operatorSpecs []spec.Operator, state *operators.PartitionState) ([]HeartbeatTimer, error) {
	var timers []HeartbeatTimer
	for _, operator := range operatorSpecs {
		if operator.Kind != "missing_heartbeat" {
			continue
		}
		operatorTimers, err := operatorHeartbeatTimers(deploymentID, tenantID, partitionID, operator, state)
		if err != nil {
			return nil, err
		}
		timers = append(timers, operatorTimers...)
	}
	return timers, nil
}

func operatorHeartbeatTimers(deploymentID, tenantID string, partitionID int, operator spec.Operator, state *operators.PartitionState) ([]HeartbeatTimer, error) {
	delay, err := spec.ParseDuration(operator.Duration)
	if err != nil {
		return nil, fmt.Errorf("parse %s duration: %w", operator.Name, err)
	}
	return stateHeartbeatTimers(deploymentID, tenantID, partitionID, operator.Name, delay, state)
}

// stateHeartbeatTimers arms a timer for each of the operator's state keys that
// holds a heartbeat.
func stateHeartbeatTimers(deploymentID, tenantID string, partitionID int, operatorID string, delay time.Duration, state *operators.PartitionState) ([]HeartbeatTimer, error) {
	var timers []HeartbeatTimer
	for stateKey, blob := range state.OperatorStates[operatorID] {
		if !hasHeartbeat(blob) {
			continue
		}
		timer, err := newHeartbeatTimer(deploymentID, tenantID, partitionID, operatorID, stateKey, blob.Heartbeat, delay)
		if err != nil {
			return nil, err
		}
		timers = append(timers, timer)
	}
	return timers, nil
}

// hasHeartbeat reports whether an operator state carries a heartbeat with a
// last event to time out.
func hasHeartbeat(blob *operators.OperatorStateBlob) bool {
	return blob != nil && blob.Heartbeat != nil && blob.Heartbeat.LastEventTime != nil
}

func newHeartbeatTimer(deploymentID, tenantID string, partitionID int, operatorID, stateKey string, heartbeat *operators.HeartbeatState, delay time.Duration) (HeartbeatTimer, error) {
	dueAt := heartbeatDueAt(heartbeat, delay)
	payload, err := json.Marshal(map[string]any{
		"operator_id": operatorID, "state_key": stateKey,
		"expected_event_id": heartbeat.LastEventID, "due_at": dueAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return HeartbeatTimer{}, fmt.Errorf("marshal heartbeat timer: %w", err)
	}
	return HeartbeatTimer{ID: heartbeatTimerID(deploymentID, tenantID, partitionID, operatorID, stateKey, dueAt),
		OperatorID: operatorID, StateKey: stateKey, DueAt: dueAt, Payload: payload}, nil
}

// heartbeatDueAt is delay after the last processing time, or after the last
// event time when no processing time was recorded.
func heartbeatDueAt(hs *operators.HeartbeatState, delay time.Duration) time.Time {
	processingTime := hs.LastProcessingTime
	if processingTime == nil {
		processingTime = hs.LastEventTime
	}
	return processingTime.Add(delay).UTC()
}

func heartbeatTimerID(deploymentID, tenantID string, partitionID int, operatorID, stateKey string, dueAt time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("agentic-stream/timer/v1\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s",
		deploymentID, tenantID, partitionID, operatorID, stateKey, "processing_time", dueAt.Format(time.RFC3339Nano))))
	return "tmr_" + hex.EncodeToString(h[:])
}
