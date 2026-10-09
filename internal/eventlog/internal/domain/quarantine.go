package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type QuarantinePayload struct {
	QuarantineID  string
	EventID       string
	EventType     string
	SchemaVersion string
	Source        string
	Payload       []byte
	Digest        []byte
}

func NewQuarantinePayload(env map[string]any) (QuarantinePayload, error) {
	payload, err := json.Marshal(env)
	if err != nil {
		return QuarantinePayload{}, fmt.Errorf("marshal quarantined payload: %w", err)
	}
	digest := contentSum(payload)
	q := QuarantinePayload{Payload: payload, Digest: digest}
	q.readHeader(env)
	if q.EventID == "" {
		q.EventID = "payload:" + hex.EncodeToString(digest[:12])
	}
	idDigest := contentSum(append([]byte(q.EventID+"|"), payload...))
	q.QuarantineID = "q_" + hex.EncodeToString(idDigest[:12])
	return q, nil
}

func (q *QuarantinePayload) readHeader(env map[string]any) {
	q.EventID, _ = env["id"].(string)
	q.EventType, _ = env["type"].(string)
	q.SchemaVersion, _ = env["schema_version"].(string)
	q.Source, _ = env["source"].(string)
}

func (q QuarantinePayload) ConflictingPayload(existingDigest []byte) bool {
	return !bytes.Equal(existingDigest, q.Digest)
}

func (q QuarantinePayload) OverflowGapID() string {
	return q.QuarantineID + ":gap"
}

func ValidQuarantine(tenantID, reason string, now time.Time) error {
	if tenantID == "" || reason == "" || now.IsZero() {
		return fmt.Errorf("tenant, reason, and time are required")
	}
	return nil
}

func ValidRelease(tenantID, eventID string, now time.Time) error {
	if tenantID == "" || eventID == "" || now.IsZero() {
		return fmt.Errorf("tenant, event, and time are required")
	}
	return nil
}

type QuarantineRecord struct {
	EventID      string `json:"event_id"`
	EventType    string `json:"event_type"`
	ReasonCode   string `json:"reason_code"`
	Status       string `json:"status"`
	AttemptCount int    `json:"attempt_count"`
	FirstSeenAt  string `json:"first_seen_at"`
	LastSeenAt   string `json:"last_seen_at"`
}

func OperatorStatus(stored string, redriven bool) string {
	if stored == "released" && redriven {
		return "redriven"
	}
	return stored
}

func contentSum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
