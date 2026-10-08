package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// QuarantinePayload is an invalid event projected onto its durable quarantine
// identity: payload bytes, payload digest, declared header fields (with a
// payload-digest fallback id) and the stable quarantine id.
type QuarantinePayload struct {
	QuarantineID  string
	EventID       string
	EventType     string
	SchemaVersion string
	Source        string
	Payload       []byte
	Digest        []byte
}

// NewQuarantinePayload marshals the invalid event and derives its identity.
// An event with no usable id is identified by its payload digest.
func NewQuarantinePayload(env map[string]any) (QuarantinePayload, error) {
	payload, err := json.Marshal(env)
	if err != nil {
		return QuarantinePayload{}, fmt.Errorf("marshal quarantined payload: %w", err)
	}
	digest := sha256.Sum256(payload)
	q := QuarantinePayload{Payload: payload, Digest: digest[:]}
	q.readHeader(env)
	if q.EventID == "" {
		q.EventID = "payload:" + hex.EncodeToString(digest[:12])
	}
	idDigest := sha256.Sum256(append([]byte(q.EventID+"|"), payload...))
	q.QuarantineID = "q_" + hex.EncodeToString(idDigest[:12])
	return q, nil
}

// readHeader copies whatever identity fields the invalid envelope carries.
func (q *QuarantinePayload) readHeader(env map[string]any) {
	q.EventID, _ = env["id"].(string)
	q.EventType, _ = env["type"].(string)
	q.SchemaVersion, _ = env["schema_version"].(string)
	q.Source, _ = env["source"].(string)
}

// ConflictingPayload reports whether an existing quarantined payload differs
// from this one under the same event id.
func (q QuarantinePayload) ConflictingPayload(existingDigest []byte) bool {
	return !bytes.Equal(existingDigest, q.Digest)
}

// OverflowGapID is the stable gap identity for a spent quarantine record.
func (q QuarantinePayload) OverflowGapID() string {
	return q.QuarantineID + ":gap"
}

// ValidQuarantine requires the fields a quarantine record cannot stand
// without.
func ValidQuarantine(tenantID, reason, now string) error {
	if tenantID == "" || reason == "" || now == "" {
		return fmt.Errorf("tenant, reason, and time are required")
	}
	return nil
}

// ValidRelease requires the fields a release or redrive cannot stand without.
func ValidRelease(tenantID, eventID, now string) error {
	if tenantID == "" || eventID == "" || now == "" {
		return fmt.Errorf("tenant, event, and time are required")
	}
	return nil
}

// QuarantineRecord is one quarantined event as an operator sees it. Status is
// quarantined, released, redriven (released and appended to the log) or
// rejected (its retries ran out; the log recorded a gap for it).
type QuarantineRecord struct {
	EventID      string `json:"event_id"`
	EventType    string `json:"event_type"`
	ReasonCode   string `json:"reason_code"`
	Status       string `json:"status"`
	AttemptCount int    `json:"attempt_count"`
	FirstSeenAt  string `json:"first_seen_at"`
	LastSeenAt   string `json:"last_seen_at"`
}

// OperatorStatus folds the redrive marker into the stored status.
func OperatorStatus(stored string, redriven bool) string {
	if stored == "released" && redriven {
		return "redriven"
	}
	return stored
}
