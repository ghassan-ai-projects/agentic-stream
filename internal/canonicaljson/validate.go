package canonicaljson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func decodeJSON(data []byte) (any, error) {
	if err := validateRawJSON(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode JSON value: %w", err)
	}
	if err := requireEnd(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

// requireEnd rejects any JSON value after the first.
func requireEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("multiple JSON values")
	}
	return fmt.Errorf("decode trailing JSON: %w", err)
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
	return requireEnd(decoder)
}

func validateTokens(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON token: %w", err)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	return validateComposite(decoder, delim)
}

func validateComposite(decoder *json.Decoder, delim json.Delim) error {
	switch delim {
	case '{':
		return validateObjectTokens(decoder)
	case '[':
		return validateArrayTokens(decoder)
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

// validateObjectTokens checks the members of an object whose opening brace
// was read: keys are strings, never repeated, and every value is valid.
func validateObjectTokens(decoder *json.Decoder) error {
	keys := make(map[string]struct{})
	for decoder.More() {
		if err := readUniqueKey(decoder, keys); err != nil {
			return err
		}
		if err := validateTokens(decoder); err != nil {
			return err
		}
	}
	return expectDelimiter(decoder, '}')
}

// readUniqueKey reads the next member key and rejects a repeated one.
func readUniqueKey(decoder *json.Decoder, keys map[string]struct{}) error {
	keyToken, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON object key: %w", err)
	}
	key, ok := keyToken.(string)
	if !ok {
		return fmt.Errorf("object key is not a string")
	}
	if _, exists := keys[key]; exists {
		return fmt.Errorf("duplicate object key %q", key)
	}
	keys[key] = struct{}{}
	return nil
}

func validateArrayTokens(decoder *json.Decoder) error {
	for decoder.More() {
		if err := validateTokens(decoder); err != nil {
			return err
		}
	}
	return expectDelimiter(decoder, ']')
}

func expectDelimiter(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read JSON delimiter: %w", err)
	}
	if token != expected {
		return fmt.Errorf("expected JSON delimiter %q, got %v", expected, token)
	}
	return nil
}

// validateStrings checks every JSON string literal for valid escapes, paired
// UTF-16 surrogates, no raw control characters, and valid UTF-8.
func validateStrings(data []byte) error {
	for i := 0; i < len(data); i++ {
		if data[i] != '"' {
			continue
		}
		end, err := scanString(data, i+1)
		if err != nil {
			return err
		}
		i = end
	}
	return nil
}

// scanString validates string content starting at i and returns the index of
// the closing quote.
func scanString(data []byte, i int) (int, error) {
	for i < len(data) {
		if data[i] == '"' {
			return i, nil
		}
		next, err := scanStringUnit(data, i)
		if err != nil {
			return 0, err
		}
		i = next
	}
	return 0, fmt.Errorf("unterminated JSON string")
}

// scanStringUnit validates one escape, control byte or UTF-8 sequence and
// returns the index after it.
func scanStringUnit(data []byte, i int) (int, error) {
	switch c := data[i]; {
	case c == '\\':
		return scanEscape(data, i+1)
	case c < 0x20:
		return 0, fmt.Errorf("unescaped control character in string")
	case c >= utf8.RuneSelf:
		return scanMultibyte(data, i)
	default:
		return i + 1, nil
	}
}

func scanMultibyte(data []byte, i int) (int, error) {
	_, size := utf8.DecodeRune(data[i:])
	if size == 1 || !utf8.Valid(data[i:i+size]) {
		return 0, fmt.Errorf("invalid UTF-8 in string")
	}
	return i + size, nil
}

// scanEscape validates the escape whose code starts at i, just after the
// backslash, and returns the index after it. A \u escape that encodes a high
// surrogate must be followed by a \u escape that encodes a low surrogate.
func scanEscape(data []byte, i int) (int, error) {
	if i >= len(data) {
		return 0, fmt.Errorf("unterminated JSON escape")
	}
	if data[i] == 'u' {
		return scanUnicodeEscape(data, i)
	}
	if !strings.ContainsRune(`"\\/bfnrt`, rune(data[i])) {
		return 0, fmt.Errorf("invalid JSON escape")
	}
	return i + 1, nil
}

// scanUnicodeEscape validates \uXXXX at i, requiring a high surrogate to be
// followed by an escaped low surrogate.
func scanUnicodeEscape(data []byte, i int) (int, error) {
	if i+4 >= len(data) {
		return 0, fmt.Errorf("short Unicode escape")
	}
	code, ok := parseHex4(data[i+1 : i+5])
	if !ok {
		return 0, fmt.Errorf("invalid Unicode escape")
	}
	if code >= 0xdc00 && code <= 0xdfff {
		return 0, fmt.Errorf("lone low surrogate")
	}
	if code < 0xd800 || code > 0xdbff {
		return i + 5, nil
	}
	return scanLowSurrogate(data, i+5)
}

func scanLowSurrogate(data []byte, i int) (int, error) {
	if i+5 >= len(data) || data[i] != '\\' || data[i+1] != 'u' {
		return 0, fmt.Errorf("lone high surrogate")
	}
	low, ok := parseHex4(data[i+2 : i+6])
	if !ok || low < 0xdc00 || low > 0xdfff {
		return 0, fmt.Errorf("invalid surrogate pair")
	}
	return i + 6, nil
}

func parseHex4(value []byte) (uint16, bool) {
	if len(value) != 4 {
		return 0, false
	}
	var result uint16
	for _, c := range value {
		digit, ok := hexDigit(c)
		if !ok {
			return 0, false
		}
		result = result<<4 + digit
	}
	return result, true
}

func hexDigit(c byte) (uint16, bool) {
	switch {
	case c >= '0' && c <= '9':
		return uint16(c - '0'), true
	case c >= 'a' && c <= 'f':
		return uint16(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return uint16(c-'A') + 10, true
	default:
		return 0, false
	}
}
