package domain

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Sealed is an admitted event with its canonical JSON and SHA-256, the
// identity under which deduplication compares payloads.
type Sealed struct {
	Event contractsv1.CloudEvent
	JSON  []byte
	SHA   []byte
}

// Seal validates the event, refuses one older than the deduplication horizon
// and canonicalizes it.
func Seal(event contractsv1.CloudEvent, now time.Time) (Sealed, error) {
	if err := event.Validate(); err != nil {
		return Sealed{}, fmt.Errorf("validate notification: %w", err)
	}
	if Expired(event.Time, now) {
		return Sealed{}, ErrEventExpired
	}
	eventJSON, err := canonicaljson.Marshal(event)
	if err != nil {
		return Sealed{}, fmt.Errorf("canonicalize notification: %w", err)
	}
	sum := sha256.Sum256(eventJSON)
	return Sealed{Event: event, JSON: eventJSON, SHA: sum[:]}, nil
}

// CheckSamePayload accepts a stored event with the same SHA-256; the same
// event id with a different payload is an error.
func CheckSamePayload(eventID string, stored, candidate []byte) error {
	if !bytes.Equal(stored, candidate) {
		return fmt.Errorf("notification event id %q has conflicting payload", eventID)
	}
	return nil
}

// RefuseRetired explains why an event identity that retention already retired
// cannot be appended again.
func RefuseRetired(eventID string, retired, candidate []byte) error {
	if !bytes.Equal(retired, candidate) {
		return fmt.Errorf("notification event id %q conflicts with tombstone", eventID)
	}
	return fmt.Errorf("notification event %q was already retired", eventID)
}
