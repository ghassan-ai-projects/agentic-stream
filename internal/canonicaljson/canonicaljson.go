// Package canonicaljson produces stable JSON serialization for digest computation.
//
// It implements a subset of RFC 8785 (JCS) sufficient for the runtime's needs:
// object keys are sorted lexicographically, arrays keep their order, and numbers
// are encoded without exponent drift for the values the runtime stores.
package canonicaljson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
)

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

// Digest computes the SHA-256 hex digest of the canonical JSON encoding of v.
func Digest(v any) (string, error) {
	b, err := Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
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
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int8:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int16:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int32:
		buf.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(x, 10))
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint64:
		buf.WriteString(strconv.FormatUint(x, 10))
	case string:
		encodeString(buf, x)
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encode(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		buf.WriteByte('{')
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			encodeString(buf, k)
			buf.WriteByte(':')
			if err := encode(buf, x[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(x, &decoded); err != nil {
			return fmt.Errorf("canonicaljson: invalid RawMessage: %w", err)
		}
		return encode(buf, decoded)
	default:
		// Fallback to standard encoding/json for structs and then re-canonicalize.
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Errorf("canonicaljson: fallback marshal: %w", err)
		}
		var decoded any
		if err := json.Unmarshal(b, &decoded); err != nil {
			return fmt.Errorf("canonicaljson: fallback unmarshal: %w", err)
		}
		return encode(buf, decoded)
	}
	return nil
}

func encodeFloat(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("canonicaljson: non-finite float %v", f)
	}
	// strconv.FormatFloat with 'f' and -1 precision avoids exponent for most
	// values; trim trailing zeros and decimal point when unnecessary.
	s := strconv.FormatFloat(f, 'f', -1, 64)
	buf.WriteString(s)
	return nil
}

func encodeString(buf *bytes.Buffer, s string) {
	// Use json.Marshal for proper escaping; it always returns a valid string.
	b, _ := json.Marshal(s)
	buf.Write(b)
}
