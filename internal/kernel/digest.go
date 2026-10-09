package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const digestPrefix = "sha256:"

// EncodeDigest formats a raw 32-byte SHA-256 sum as the "sha256:<hex>" text
// that contracts, documents and schemas carry.
func EncodeDigest(sum []byte) string {
	return digestPrefix + hex.EncodeToString(sum)
}

// DecodeDigest converts "sha256:<hex>" text into its 32-byte storage form.
// Unprefixed digests, uppercase hex and any other length are invalid.
func DecodeDigest(digest string) ([]byte, error) {
	if !strings.HasPrefix(digest, digestPrefix) {
		return nil, fmt.Errorf("digest must use %q prefix", digestPrefix)
	}
	encoded := strings.TrimPrefix(digest, digestPrefix)
	if encoded != strings.ToLower(encoded) {
		return nil, fmt.Errorf("digest must use lowercase hex")
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode digest: %w", err)
	}
	if len(decoded) != sha256.Size {
		return nil, fmt.Errorf("digest has %d bytes, want %d", len(decoded), sha256.Size)
	}
	return decoded, nil
}
