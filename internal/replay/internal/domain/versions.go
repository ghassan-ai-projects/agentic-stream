package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// VersionDigest is one situation-version snapshot digest in canonical order.
type VersionDigest struct {
	SituationID string
	Version     int
	SHA256      []byte
}

// HashVersionDigests produces the deterministic canonical hash over ordered
// version digests: the situations hash of a replay result.
func HashVersionDigests(versions []VersionDigest) string {
	h := sha256.New()
	for _, v := range versions {
		_, _ = fmt.Fprintf(h, "%s%d", v.SituationID, v.Version)
		_, _ = h.Write(v.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}
