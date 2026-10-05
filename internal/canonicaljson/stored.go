package canonicaljson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// EncodeDigest formats a raw 32-byte SHA-256 sum as a "sha256:<hex>"
// reference. It is the inverse of DecodeDigest.
func EncodeDigest(sum []byte) string {
	return digestPrefix + hex.EncodeToString(sum)
}

// ContentDigest returns the "sha256:<hex>" reference of data's raw SHA-256,
// without a domain prefix. It identifies stored canonical documents.
func ContentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return EncodeDigest(sum[:])
}

// VerifyStored checks a stored canonical JSON document against its raw
// SHA-256. A non-canonical document is reported before a digest mismatch, so
// readers fail closed on tampered or re-encoded evidence.
func VerifyStored(data, digest []byte) error {
	if len(digest) != sha256.Size {
		return fmt.Errorf("stored digest has %d bytes", len(digest))
	}
	if err := checkCanonical(data); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if !bytes.Equal(sum[:], digest) {
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
