package canonicaljson

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

const maxSafeInteger = uint64(1<<53 - 1)

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
	digits, ok := scaleDigits(whole+fraction, len(fraction)-exponent)
	if !ok {
		return nil, false
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		digits = "0"
	}
	integer, ok := new(big.Int).SetString(sign+digits, 10)
	return integer, ok
}

// scaleDigits shifts a decimal digit string left by decimalPlaces (right when
// negative). It reports false when the shift drops a nonzero digit, meaning
// the number is not an integer.
func scaleDigits(digits string, decimalPlaces int) (string, bool) {
	switch {
	case decimalPlaces < 0:
		return digits + strings.Repeat("0", -decimalPlaces), true
	case decimalPlaces == 0:
		return digits, true
	case decimalPlaces >= len(digits):
		return "0", strings.Trim(digits, "0") == ""
	default:
		cut := len(digits) - decimalPlaces
		return digits[:cut], strings.Trim(digits[cut:], "0") == ""
	}
}

func integerRoundTrips(want *big.Int, f float64) bool {
	if math.Trunc(f) != f {
		return false
	}
	got, _ := new(big.Float).SetFloat64(f).Int(nil)
	return want.Cmp(got) == 0
}
