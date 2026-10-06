package domain

import (
	"bytes"
	"cmp"
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
	if err := checkExactInteger(raw, f); err != nil {
		return err
	}
	return encodeFloat(buf, f)
}

// checkExactInteger requires an integer-valued literal to be a safe integer
// that the parsed float represents exactly.
func checkExactInteger(raw string, f float64) error {
	integer, ok := decimalInteger(raw)
	if !ok {
		return nil
	}
	if new(big.Int).Abs(integer).Cmp(new(big.Int).SetUint64(maxSafeInteger)) > 0 {
		return fmt.Errorf("canonicaljson: unsafe JSON integer %q", raw)
	}
	if !integerRoundTrips(integer, f) {
		return fmt.Errorf("canonicaljson: JSON integer %q does not round-trip", raw)
	}
	return nil
}

func encodeFloat(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("canonicaljson: non-finite float %v", f)
	}
	if f == 0 && math.Signbit(f) {
		return fmt.Errorf("canonicaljson: negative zero is not allowed")
	}
	formatted, err := formatFloat(f)
	if err != nil {
		return err
	}
	buf.WriteString(formatted)
	return nil
}

// formatFloat is the shortest round-trip form, written without an exponent
// for exponents in [-6, 20] and with a normalized exponent otherwise.
func formatFloat(f float64) (string, error) {
	if f == 0 {
		return "0", nil
	}
	shortest := strconv.FormatFloat(f, 'g', -1, 64)
	mantissa, exponentText, hasExponent := strings.Cut(shortest, "e")
	if !hasExponent {
		return shortest, nil
	}
	exponent, err := strconv.Atoi(exponentText)
	if err != nil {
		return "", fmt.Errorf("canonicaljson: parse float exponent %q: %w", shortest, err)
	}
	return placeExponent(mantissa, exponent), nil
}

// placeExponent writes exponents in [-6, 20] positionally and normalizes the
// others.
func placeExponent(mantissa string, exponent int) string {
	if exponent >= -6 && exponent <= 20 {
		return expandExponent(mantissa, exponent)
	}
	return normalizeExponent(mantissa, exponent)
}

func expandExponent(mantissa string, exponent int) string {
	sign, mantissa := splitSign(mantissa)
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

// splitSign separates a leading minus sign from a decimal literal.
func splitSign(value string) (string, string) {
	if rest, negative := strings.CutPrefix(value, "-"); negative {
		return "-", rest
	}
	return "", value
}

func normalizeExponent(mantissa string, exponent int) string {
	if exponent >= 0 {
		return mantissa + "e+" + strconv.Itoa(exponent)
	}
	return mantissa + "e" + strconv.Itoa(exponent)
}

func decimalInteger(raw string) (*big.Int, bool) {
	if strings.HasPrefix(raw, "+") {
		return nil, false
	}
	sign, raw := splitSign(raw)
	mantissa, exponent, ok := splitExponent(raw)
	if !ok {
		return nil, false
	}
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits, ok := scaleDigits(whole+fraction, len(fraction)-exponent)
	if !ok {
		return nil, false
	}
	return new(big.Int).SetString(sign+cmp.Or(strings.TrimLeft(digits, "0"), "0"), 10)
}

// splitExponent separates a decimal literal's mantissa from its exponent.
func splitExponent(raw string) (string, int, bool) {
	index := strings.IndexAny(raw, "eE")
	if index < 0 {
		return raw, 0, true
	}
	exponent, err := strconv.Atoi(raw[index+1:])
	if err != nil {
		return "", 0, false
	}
	return raw[:index], exponent, true
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
