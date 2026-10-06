package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// EncodedEvent is an envelope's stored JSON columns: the payload, its SHA-256
// digest and the quality object.
type EncodedEvent struct {
	PayloadJSON   []byte
	PayloadSHA256 []byte
	QualityJSON   []byte
}

// EncodeEventBody marshals the payload and quality and digests the payload
// bytes exactly as they are stored.
func EncodeEventBody(data map[string]any, quality any) (EncodedEvent, error) {
	payloadJSON, err := json.Marshal(data)
	if err != nil {
		return EncodedEvent{}, fmt.Errorf("marshal payload: %w", err)
	}
	payloadHash := sha256.Sum256(payloadJSON)
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return EncodedEvent{}, fmt.Errorf("marshal quality: %w", err)
	}
	return EncodedEvent{PayloadJSON: payloadJSON, PayloadSHA256: payloadHash[:], QualityJSON: qualityJSON}, nil
}

// StoredTimes are one stored record's time columns as text, before parsing.
type StoredTimes struct {
	EventTime, IngestedAt string
	ObservedAt            *string
}

// ParseStoredTimes parses the stored time columns in column order.
func (s StoredTimes) Parse() (time.Time, time.Time, *time.Time, error) {
	eventTime, err := time.Parse(time.RFC3339Nano, s.EventTime)
	if err != nil {
		return time.Time{}, time.Time{}, nil, fmt.Errorf("parse event_time: %w", err)
	}
	ingestedAt, err := time.Parse(time.RFC3339Nano, s.IngestedAt)
	if err != nil {
		return time.Time{}, time.Time{}, nil, fmt.Errorf("parse ingested_at: %w", err)
	}
	observedAt, err := parseObservedAt(s.ObservedAt)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	return eventTime, ingestedAt, observedAt, nil
}

func parseObservedAt(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil, fmt.Errorf("parse observed_at: %w", err)
	}
	return &parsed, nil
}

// ReadLimit is the effective record-read limit for a request: a positive
// request limit, else the default of 1000.
func ReadLimit(limit int) int {
	if limit > 0 {
		return limit
	}
	return 1000
}
