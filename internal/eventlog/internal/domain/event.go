package domain

import (
	"encoding/json"
	"fmt"
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
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return EncodedEvent{}, fmt.Errorf("marshal quality: %w", err)
	}
	return EncodedEvent{PayloadJSON: payloadJSON, PayloadSHA256: contentSum(payloadJSON), QualityJSON: qualityJSON}, nil
}

// ReadLimit is the effective record-read limit for a request: a positive
// request limit, else the default of 1000.
func ReadLimit(limit int) int {
	if limit > 0 {
		return limit
	}
	return 1000
}
