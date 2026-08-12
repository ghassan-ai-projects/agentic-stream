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
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Domain identifies the contract namespace included in a digest preimage.
// The newline is part of every domain and prevents concatenation ambiguity.
type Domain string

const (
	DomainSnapshot Domain = "situation-runtime/snapshot/v1\n"
	DomainSpec     Domain = "situation-runtime/spec/v1\n"
	DomainDecision Domain = "situation-runtime/decision/v1\n"
	DomainIntent   Domain = "situation-runtime/intent/v1\n"
	DomainCommand  Domain = "situation-runtime/command/v1\n"
	DomainEvent    Domain = "situation-runtime/event/v1\n"
	DomainOutcome  Domain = "situation-runtime/outcome/v1\n"
	DomainEnvelope Domain = "situation-runtime/envelope/v1\n"
	DomainTest     Domain = "situation-runtime/test/v1\n"
)

const maxSafeInteger = uint64(1<<53 - 1)

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
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case float64:
		return encodeFloat(buf, x)
	case float32:
		return encodeFloat(buf, float64(x))
	case int:
		return encodeInteger(buf, int64(x))
	case int8:
		return encodeInteger(buf, int64(x))
	case int16:
		return encodeInteger(buf, int64(x))
	case int32:
		return encodeInteger(buf, int64(x))
	case int64:
		return encodeInteger(buf, x)
	case uint:
		return encodeUnsigned(buf, uint64(x))
	case uint8:
		return encodeUnsigned(buf, uint64(x))
	case uint16:
		return encodeUnsigned(buf, uint64(x))
	case uint32:
		return encodeUnsigned(buf, uint64(x))
	case uint64:
		return encodeUnsigned(buf, x)
	case json.Number:
		return encodeNumber(buf, string(x))
	case string:
		return encodeString(buf, x)
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
	return nil
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

func encodeInteger(buf *bytes.Buffer, value int64) error {
	if value > int64(maxSafeInteger) || value < -int64(maxSafeInteger) {
		return fmt.Errorf("canonicaljson: integer %d exceeds exact JSON number range", value)
	}
	buf.WriteString(strconv.FormatInt(value, 10))
	return nil
}

func encodeUnsigned(buf *bytes.Buffer, value uint64) error {
	if value > maxSafeInteger {
		return fmt.Errorf("canonicaljson: integer %d exceeds exact JSON number range", value)
	}
	buf.WriteString(strconv.FormatUint(value, 10))
	return nil
}

func encodeNumber(buf *bytes.Buffer, raw string) error {
	if raw == "" {
		return fmt.Errorf("canonicaljson: empty JSON number")
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return fmt.Errorf("canonicaljson: invalid JSON number %q", raw)
	}
	if f == 0 && strings.HasPrefix(raw, "-") {
		return fmt.Errorf("canonicaljson: negative zero is not allowed")
	}
	if integer, ok := decimalInteger(raw); ok {
		limit := new(big.Int).SetUint64(maxSafeInteger)
		if new(big.Int).Abs(integer).Cmp(limit) > 0 {
			return fmt.Errorf("canonicaljson: unsafe JSON integer %q", raw)
		}
		if !integerRoundTrips(integer, f) {
			return fmt.Errorf("canonicaljson: JSON integer %q does not round-trip", raw)
		}
	}
	return encodeFloat(buf, f)
}

func encodeFloat(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("canonicaljson: non-finite float %v", f)
	}
	if f == 0 {
		if math.Signbit(f) {
			return fmt.Errorf("canonicaljson: negative zero is not allowed")
		}
		buf.WriteByte('0')
		return nil
	}

	shortest := strconv.FormatFloat(f, 'g', -1, 64)
	if exponentIndex := strings.IndexByte(shortest, 'e'); exponentIndex >= 0 {
		mantissa := shortest[:exponentIndex]
		exponent, err := strconv.Atoi(shortest[exponentIndex+1:])
		if err != nil {
			return fmt.Errorf("canonicaljson: parse float exponent %q: %w", shortest, err)
		}
		if exponent >= -6 && exponent <= 20 {
			shortest = expandExponent(mantissa, exponent)
		} else {
			shortest = normalizeExponent(mantissa, exponent)
		}
	}
	buf.WriteString(shortest)
	return nil
}

func expandExponent(mantissa string, exponent int) string {
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign = "-"
		mantissa = mantissa[1:]
	}
	dot := strings.IndexByte(mantissa, '.')
	if dot < 0 {
		dot = len(mantissa)
	}
	digits := strings.ReplaceAll(mantissa, ".", "")
	position := dot + exponent
	switch {
	case position <= 0:
		return sign + "0." + strings.Repeat("0", -position) + digits
	case position >= len(digits):
		return sign + digits + strings.Repeat("0", position-len(digits))
	default:
		return sign + digits[:position] + "." + digits[position:]
	}
}

func normalizeExponent(mantissa string, exponent int) string {
	if exponent >= 0 {
		return mantissa + "e+" + strconv.Itoa(exponent)
	}
	return mantissa + "e" + strconv.Itoa(exponent)
}

func encodeString(buf *bytes.Buffer, value string) error {
	if !utf8.ValidString(value) || containsSurrogate(value) {
		return fmt.Errorf("canonicaljson: string is not well-formed Unicode")
	}
	buf.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			buf.WriteByte('\\')
			buf.WriteRune(r)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
				continue
			}
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
	return nil
}

func containsSurrogate(value string) bool {
	for _, r := range value {
		if r >= 0xd800 && r <= 0xdfff {
			return true
		}
	}
	return false
}

func compareUTF16(a, b string) int {
	a16 := utf16Units(a)
	b16 := utf16Units(b)
	for i := 0; i < len(a16) && i < len(b16); i++ {
		if a16[i] < b16[i] {
			return -1
		}
		if a16[i] > b16[i] {
			return 1
		}
	}
	switch {
	case len(a16) < len(b16):
		return -1
	case len(a16) > len(b16):
		return 1
	default:
		return 0
	}
}

func utf16Units(value string) []uint16 {
	units := make([]uint16, 0, len(value))
	for _, r := range value {
		if r <= 0xffff {
			units = append(units, uint16(r))
			continue
		}
		r -= 0x10000
		units = append(units, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff)))
	}
	return units
}

func decimalInteger(raw string) (*big.Int, bool) {
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = raw[1:]
	} else if strings.HasPrefix(raw, "+") {
		return nil, false
	}
	exponent := 0
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		parsed, err := strconv.Atoi(raw[index+1:])
		if err != nil {
			return nil, false
		}
		exponent = parsed
		raw = raw[:index]
	}
	parts := strings.SplitN(raw, ".", 2)
	whole := parts[0]
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	digits := whole + fraction
	decimalPlaces := len(fraction) - exponent
	if decimalPlaces > 0 {
		if decimalPlaces >= len(digits) {
			if strings.Trim(digits, "0") != "" {
				return nil, false
			}
			digits = "0"
		} else {
			cut := len(digits) - decimalPlaces
			if strings.Trim(digits[cut:], "0") != "" {
				return nil, false
			}
			digits = digits[:cut]
		}
	} else if decimalPlaces < 0 {
		digits += strings.Repeat("0", -decimalPlaces)
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		digits = "0"
	}
	integer, ok := new(big.Int).SetString(sign+digits, 10)
	return integer, ok
}

func integerRoundTrips(want *big.Int, f float64) bool {
	if math.Trunc(f) != f {
		return false
	}
	got, _ := new(big.Float).SetFloat64(f).Int(nil)
	return want.Cmp(got) == 0
}

func decodeJSON(data []byte) (any, error) {
	if err := validateRawJSON(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func validateRawJSON(data []byte) error {
	if err := validateStrings(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateTokens(decoder); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateTokens(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate object key %q", key)
			}
			keys[key] = struct{}{}
			if err := validateTokens(decoder); err != nil {
				return err
			}
		}
		return expectDelimiter(decoder, '}')
	case '[':
		for decoder.More() {
			if err := validateTokens(decoder); err != nil {
				return err
			}
		}
		return expectDelimiter(decoder, ']')
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

func expectDelimiter(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != expected {
		return fmt.Errorf("expected JSON delimiter %q, got %v", expected, token)
	}
	return nil
}

func validateStrings(data []byte) error {
	for i := 0; i < len(data); i++ {
		if data[i] != '"' {
			continue
		}
		i++
		for i < len(data) {
			switch data[i] {
			case '"':
				goto nextString
			case '\\':
				i++
				if i >= len(data) {
					return fmt.Errorf("unterminated JSON escape")
				}
				if data[i] != 'u' {
					if !strings.ContainsRune(`"\\/bfnrt`, rune(data[i])) {
						return fmt.Errorf("invalid JSON escape")
					}
					i++
					continue
				}
				if i+4 >= len(data) {
					return fmt.Errorf("short Unicode escape")
				}
				code, ok := parseHex4(data[i+1 : i+5])
				if !ok {
					return fmt.Errorf("invalid Unicode escape")
				}
				i += 5
				if code >= 0xdc00 && code <= 0xdfff {
					return fmt.Errorf("lone low surrogate")
				}
				if code >= 0xd800 && code <= 0xdbff {
					if i+5 >= len(data) || data[i] != '\\' || data[i+1] != 'u' {
						return fmt.Errorf("lone high surrogate")
					}
					low, ok := parseHex4(data[i+2 : i+6])
					if !ok || low < 0xdc00 || low > 0xdfff {
						return fmt.Errorf("invalid surrogate pair")
					}
					i += 6
				}
			default:
				if data[i] < 0x20 {
					return fmt.Errorf("unescaped control character in string")
				}
				if c := data[i]; c >= utf8.RuneSelf {
					_, size := utf8.DecodeRune(data[i:])
					if size == 1 || !utf8.Valid(data[i:i+size]) {
						return fmt.Errorf("invalid UTF-8 in string")
					}
					i += size
					continue
				}
				i++
			}
		}
		return fmt.Errorf("unterminated JSON string")
	nextString:
	}
	return nil
}

func parseHex4(value []byte) (uint16, bool) {
	if len(value) != 4 {
		return 0, false
	}
	var result uint16
	for _, c := range value {
		result <<= 4
		switch {
		case c >= '0' && c <= '9':
			result += uint16(c - '0')
		case c >= 'a' && c <= 'f':
			result += uint16(c-'a') + 10
		case c >= 'A' && c <= 'F':
			result += uint16(c-'A') + 10
		default:
			return 0, false
		}
	}
	return result, true
}
