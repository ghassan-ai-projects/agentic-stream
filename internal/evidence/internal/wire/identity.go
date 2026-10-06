package wire

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// TokenID assigns an opaque identifier when none was supplied.
func TokenID(existing string) (string, error) {
	if existing != "" {
		return existing, nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate capability token id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func NewRuntimeEpoch() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate runtime epoch: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
