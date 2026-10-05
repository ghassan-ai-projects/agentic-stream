package canonicaljson

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestDigestEncodingRoundTrips(t *testing.T) {
	t.Parallel()
	reference := ContentDigest([]byte(`{"a":1}`))
	raw, err := DecodeDigest(reference)
	if err != nil {
		t.Fatal(err)
	}
	if EncodeDigest(raw) != reference {
		t.Fatalf("round trip = %s, want %s", EncodeDigest(raw), reference)
	}
}

func TestVerifyStoredChecksCanonicalFormBeforeDigest(t *testing.T) {
	t.Parallel()
	canonical := []byte(`{"value":1}`)
	sum := sha256.Sum256(canonical)
	wrong := sha256.Sum256([]byte(`{}`))
	tests := []struct {
		name   string
		data   []byte
		digest []byte
		want   string
	}{
		{"valid", canonical, sum[:], ""},
		{"short digest", canonical, sum[:4], "stored digest has 4 bytes"},
		{"invalid json", []byte(`{`), sum[:], "stored JSON is invalid"},
		{"non-canonical before mismatch", []byte(` {"value":1}`), wrong[:], "stored JSON is not canonical"},
		{"digest mismatch", canonical, wrong[:], "stored JSON digest mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := VerifyStored(tt.data, tt.digest)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("VerifyStored = %v, want %q", err, tt.want)
			}
		})
	}
}
