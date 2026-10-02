// Package canonicaljson produces JCS-compatible JSON for contract identity and
// digest computation, with the runtime's strict producer validation profile.
package canonicaljson

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Domain identifies the contract namespace included in a digest preimage.
// The newline is part of every domain and prevents concatenation ambiguity.
type Domain string

const (
	DomainSnapshot          Domain = "situation-runtime/snapshot/v1\n"
	DomainSpec              Domain = "situation-runtime/spec/v1\n"
	DomainDecision          Domain = "situation-runtime/decision/v1\n"
	DomainIntent            Domain = "situation-runtime/intent/v1\n"
	DomainCommand           Domain = "situation-runtime/command/v1\n"
	DomainEvent             Domain = "situation-runtime/event/v1\n"
	DomainOutcome           Domain = "situation-runtime/outcome/v1\n"
	DomainSituationState    Domain = "situation-runtime/situation-state/v1\n"
	DomainEnvelope          Domain = "situation-runtime/envelope/v1\n"
	DomainPrompt            Domain = "situation-runtime/prompt/v1\n"
	DomainObjective         Domain = "situation-runtime/objective/v1\n"
	DomainDiagnosisCatalog  Domain = "situation-runtime/diagnosis-catalog/v1\n"
	DomainIntentCatalog     Domain = "situation-runtime/intent-catalog/v1\n"
	DomainApproval          Domain = "situation-runtime/approval-assertion/v1\n"
	DomainPolicy            Domain = "situation-runtime/policy/v1\n"
	DomainCapabilityCatalog Domain = "situation-runtime/capability-catalog/v1\n"
	DomainShadowComparison  Domain = "situation-runtime/shadow-comparison/v1\n"
	DomainTest              Domain = "situation-runtime/test/v1\n"
)

const digestPrefix = "sha256:"

// Marshal returns the canonical JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encode(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalString returns the canonical JSON string.
func MarshalString(v any) (string, error) {
	b, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Digest computes a domain-separated SHA-256 digest of v.
func Digest(domain Domain, v any) (string, error) {
	if domain == "" {
		return "", fmt.Errorf("canonicaljson: empty digest domain")
	}
	b, err := Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write(b)
	return digestPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

// DecodeDigest converts a canonical sha256 digest into its 32-byte storage
// representation. Unprefixed digests are invalid contract values.
func DecodeDigest(digest string) ([]byte, error) {
	if !strings.HasPrefix(digest, digestPrefix) {
		return nil, fmt.Errorf("canonicaljson: digest must use %q prefix", digestPrefix)
	}
	encoded := strings.TrimPrefix(digest, digestPrefix)
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("canonicaljson: decode digest: %w", err)
	}
	if len(decoded) != sha256.Size {
		return nil, fmt.Errorf("canonicaljson: digest has %d bytes, want %d", len(decoded), sha256.Size)
	}
	return decoded, nil
}

// Verify recomputes a domain-separated digest and compares it in constant
// time. It returns false for malformed or non-canonical values.
func Verify(domain Domain, v any, digest string) bool {
	expected, err := Digest(domain, v)
	if err != nil || len(expected) != len(digest) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(digest)) == 1
}

func encode(buf *bytes.Buffer, v any) error {
	if handled, err := encodeScalar(buf, v); handled {
		return err
	}
	switch x := v.(type) {
	case []any:
		return encodeArray(buf, x)
	case map[string]any:
		return encodeObject(buf, x)
	case json.RawMessage:
		decoded, err := decodeJSON(x)
		if err != nil {
			return fmt.Errorf("canonicaljson: invalid RawMessage: %w", err)
		}
		return encode(buf, decoded)
	default:
		// Marshal structs and typed collections once, then canonicalize the
		// resulting JSON with duplicate-key and Unicode validation enabled.
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Errorf("canonicaljson: fallback marshal: %w", err)
		}
		decoded, err := decodeJSON(b)
		if err != nil {
			return fmt.Errorf("canonicaljson: fallback unmarshal: %w", err)
		}
		return encode(buf, decoded)
	}
}

// encodeScalar encodes null, booleans, strings, and numbers. It reports
// whether v was one of those.
func encodeScalar(buf *bytes.Buffer, v any) (bool, error) {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		return true, encodeString(buf, x)
	case json.Number:
		return true, encodeNumber(buf, string(x))
	default:
		return encodeGoNumber(buf, v)
	}
	return true, nil
}

// encodeGoNumber encodes Go's built-in numeric types. It reports whether v
// was one of them.
func encodeGoNumber(buf *bytes.Buffer, v any) (bool, error) {
	switch x := v.(type) {
	case float64:
		return true, encodeFloat(buf, x)
	case float32:
		return true, encodeFloat(buf, float64(x))
	case int:
		return true, encodeInteger(buf, int64(x))
	case int8:
		return true, encodeInteger(buf, int64(x))
	case int16:
		return true, encodeInteger(buf, int64(x))
	case int32:
		return true, encodeInteger(buf, int64(x))
	case int64:
		return true, encodeInteger(buf, x)
	case uint:
		return true, encodeUnsigned(buf, uint64(x))
	case uint8:
		return true, encodeUnsigned(buf, uint64(x))
	case uint16:
		return true, encodeUnsigned(buf, uint64(x))
	case uint32:
		return true, encodeUnsigned(buf, uint64(x))
	case uint64:
		return true, encodeUnsigned(buf, x)
	default:
		return false, nil
	}
}

func encodeArray(buf *bytes.Buffer, values []any) error {
	buf.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := encode(buf, value); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

func encodeObject(buf *bytes.Buffer, values map[string]any) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !utf8.ValidString(key) {
			return fmt.Errorf("canonicaljson: invalid UTF-8 object key")
		}
		if containsSurrogate(key) {
			return fmt.Errorf("canonicaljson: object key contains surrogate")
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareUTF16(keys[i], keys[j]) < 0
	})

	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := encodeString(buf, key); err != nil {
			return err
		}
		buf.WriteByte(':')
		if err := encode(buf, values[key]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}
