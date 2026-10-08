package domain

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
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
)

// Marshal returns the canonical JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encode(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Digest computes a domain-separated SHA-256 digest of v.
func Digest(domain Domain, v any) (string, error) {
	sum, err := DigestSum(domain, v)
	if err != nil {
		return "", err
	}
	return EncodeDigest(sum), nil
}

// DigestSum computes the raw 32-byte domain-separated SHA-256 digest of v.
func DigestSum(domain Domain, v any) ([]byte, error) {
	_, sum, err := Seal(domain, v)
	return sum, err
}

// Seal returns the canonical JSON of v and its raw domain-separated digest,
// canonicalizing v once.
func Seal(domain Domain, v any) ([]byte, []byte, error) {
	if domain == "" {
		return nil, nil, fmt.Errorf("canonicaljson: empty digest domain")
	}
	canonical, err := Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write(canonical)
	return canonical, h.Sum(nil), nil
}

// VerifySum recomputes the domain-separated digest of v and compares it with
// sum in constant time. It returns false for malformed or non-canonical values.
func VerifySum(domain Domain, v any, sum []byte) bool {
	expected, err := DigestSum(domain, v)
	if err != nil || len(expected) != len(sum) {
		return false
	}
	return subtle.ConstantTimeCompare(expected, sum) == 1
}

// DecodeDigest converts a canonical sha256 digest into its 32-byte storage
// representation. Unprefixed digests and uppercase hex are invalid contract
// values.
func DecodeDigest(digest string) ([]byte, error) {
	decoded, err := kernel.DecodeDigest(digest)
	if err != nil {
		return nil, fmt.Errorf("canonicaljson: %w", err)
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
		return encodeRaw(buf, x)
	default:
		return encodeMarshaled(buf, x)
	}
}

func encodeRaw(buf *bytes.Buffer, raw json.RawMessage) error {
	decoded, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("canonicaljson: invalid RawMessage: %w", err)
	}
	return encode(buf, decoded)
}

// encodeMarshaled marshals structs and typed collections once, then
// canonicalizes the resulting JSON with duplicate-key and Unicode validation.
func encodeMarshaled(buf *bytes.Buffer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("canonicaljson: fallback marshal: %w", err)
	}
	decoded, err := decodeJSON(b)
	if err != nil {
		return fmt.Errorf("canonicaljson: fallback unmarshal: %w", err)
	}
	return encode(buf, decoded)
}

// encodeScalar encodes null, booleans, strings, and numbers. It reports
// whether v was one of those.
func encodeScalar(buf *bytes.Buffer, v any) (bool, error) {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
		return true, nil
	case bool:
		buf.WriteString(strconv.FormatBool(x))
		return true, nil
	case string:
		return true, encodeString(buf, x)
	case json.Number:
		return true, encodeNumber(buf, string(x))
	default:
		return encodeGoNumber(buf, v)
	}
}

// encodeGoNumber encodes Go's built-in numeric types. It reports whether v
// was one of them.
func encodeGoNumber(buf *bytes.Buffer, v any) (bool, error) {
	switch x := v.(type) {
	case float64:
		return true, encodeFloat(buf, x)
	case float32:
		return true, encodeFloat(buf, float64(x))
	}
	if signed, ok := signedInteger(v); ok {
		return true, encodeInteger(buf, signed)
	}
	if unsigned, ok := unsignedInteger(v); ok {
		return true, encodeUnsigned(buf, unsigned)
	}
	return false, nil
}

func signedInteger(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	}
	return 0, false
}

func unsignedInteger(v any) (uint64, bool) {
	switch x := v.(type) {
	case uint:
		return uint64(x), true
	case uint8:
		return uint64(x), true
	case uint16:
		return uint64(x), true
	case uint32:
		return uint64(x), true
	case uint64:
		return x, true
	}
	return 0, false
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
	keys, err := sortedKeys(values)
	if err != nil {
		return err
	}
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := encodeMember(buf, key, values[key]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// sortedKeys validates the object keys and orders them by UTF-16 code units.
func sortedKeys(values map[string]any) ([]string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !utf8.ValidString(key) {
			return nil, fmt.Errorf("canonicaljson: invalid UTF-8 object key")
		}
		if containsSurrogate(key) {
			return nil, fmt.Errorf("canonicaljson: object key contains surrogate")
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return compareUTF16(keys[i], keys[j]) < 0 })
	return keys, nil
}

func encodeMember(buf *bytes.Buffer, key string, value any) error {
	if err := encodeString(buf, key); err != nil {
		return err
	}
	buf.WriteByte(':')
	return encode(buf, value)
}
