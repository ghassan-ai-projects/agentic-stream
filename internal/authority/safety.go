package authority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// SafetyEvent is a durable input to the soak verdict. It is intentionally
// explicit; arbitrary telemetry labels cannot silently become safety claims.
type SafetyEvent struct {
	Type      string
	Target    string
	CommandID string
	Details   map[string]any
	Occurred  time.Time
}

// SafetyLedger records events from the emulator or physical evidence bridge.
// The runtime does not synthesize physical transitions from a receipt.
type SafetyLedger struct{ DB *storage.DB }

// Record appends one validated safety event.
func (l *SafetyLedger) Record(ctx context.Context, event SafetyEvent) error {
	if l == nil || l.DB == nil || event.Target == "" || !validSafetyEventType(event.Type) {
		return fmt.Errorf("safety event type, target, and ledger are required")
	}
	if event.Occurred.IsZero() {
		event.Occurred = time.Now().UTC()
	}
	if event.Details == nil {
		event.Details = map[string]any{}
	}
	if event.Type == "physical_transition" && eventComplete(event.Details) && !PhysicalEvidenceComplete(event.Details) {
		return fmt.Errorf("complete physical transition evidence requires source and sha256 evidence_digest")
	}
	return l.storeSafetyEvent(ctx, event)
}

func validSafetyEventType(eventType string) bool {
	switch eventType {
	case "unsafe_output", "stale_energizing_effect", "duplicate_net_energizing_effect", "unexplained_actuator_transition", "false_verified_success", "safe_state_deadline_miss", "physical_transition":
		return true
	default:
		return false
	}
}

// PhysicalEvidenceComplete reports whether a physical transition carries the
// minimum provenance shape required for a complete run artifact. This validates
// evidence metadata, not the truth of the physical observation; an independent
// feedback system must still supply and own that evidence.
func PhysicalEvidenceComplete(details map[string]any) bool {
	if !eventComplete(details) {
		return false
	}
	source, _ := details["source"].(string)
	digest, _ := details["evidence_digest"].(string)
	return source != "" && validSHA256Reference(digest)
}

func eventComplete(details map[string]any) bool {
	complete, _ := details["evidence_complete"].(bool)
	return complete
}

func validSHA256Reference(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func (l *SafetyLedger) storeSafetyEvent(ctx context.Context, event SafetyEvent) error {
	data, err := canonicaljson.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("canonicalize safety event: %w", err)
	}
	hash := sha256.Sum256(data)
	if _, err := l.DB.ExecContext(ctx, recordDeviceSafetyEventSQL, event.Type, event.Target, nullableText(event.CommandID), data, hash[:], formatRuntimeTime(event.Occurred)); err != nil {
		return fmt.Errorf("record safety event: %w", err)
	}
	return nil
}

const recordDeviceSafetyEventSQL = `
		INSERT INTO device_safety_events
			(event_type, target, command_id, details_json, details_sha256, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)`
