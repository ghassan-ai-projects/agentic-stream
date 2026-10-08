package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// EncodeDigest formats a raw 32-byte SHA-256 sum as a "sha256:<hex>"
// reference. It is the inverse of DecodeDigest.
func EncodeDigest(sum []byte) string {
	return kernel.EncodeDigest(sum)
}

// ContentDigest returns the "sha256:<hex>" reference of data's raw SHA-256,
// without a domain prefix. It identifies stored canonical documents.
func ContentDigest(data []byte) string {
	return EncodeDigest(Sum(data))
}

// Sum returns the raw 32-byte SHA-256 of data, the content hash of stored
// bytes.
func Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

// HasSumLength reports whether b is a complete 32-byte SHA-256 sum.
func HasSumLength(b []byte) bool {
	return len(b) == sha256.Size
}

// VerifyStored checks a stored canonical JSON document against its raw
// SHA-256. A non-canonical document is reported before a digest mismatch, so
// readers fail closed on tampered or re-encoded evidence.
func VerifyStored(data, digest []byte) error {
	if !HasSumLength(digest) {
		return fmt.Errorf("stored digest has %d bytes", len(digest))
	}
	if err := checkCanonical(data); err != nil {
		return err
	}
	if !bytes.Equal(Sum(data), digest) {
		return fmt.Errorf("stored JSON digest mismatch")
	}
	return nil
}

func checkCanonical(data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("stored JSON is invalid: %w", err)
	}
	canonical, err := Marshal(value)
	if err != nil || !bytes.Equal(canonical, data) {
		return fmt.Errorf("stored JSON is not canonical")
	}
	return nil
}
