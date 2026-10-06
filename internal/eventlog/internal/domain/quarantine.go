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

// Gap is one durable discontinuity: an ordered position span in a partition,
// with the reason it was recorded and the recording time.
type Gap struct {
	ID          string
	TenantID    string
	PartitionID int
	From, To    int64
	Reason      string
	CreatedAt   string
}

// Valid requires a coherent gap interval: ordered positions, a partition and
// a reason and time.
func (g Gap) Valid() error {
	if g.ID == "" || g.TenantID == "" || g.PartitionID < 0 || g.From < 0 || g.To < g.From || g.Reason == "" || g.CreatedAt == "" {
		return fmt.Errorf("invalid event gap")
	}
	return nil
}
