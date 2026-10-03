package canonicaljson

import (
	"bytes"
	"fmt"
	"slices"
	"unicode/utf8"
)

func encodeString(buf *bytes.Buffer, value string) error {
	if !utf8.ValidString(value) || containsSurrogate(value) {
		return fmt.Errorf("canonicaljson: string is not well-formed Unicode")
	}
	buf.WriteByte('"')
	for _, r := range value {
		writeStringRune(buf, r)
	}
	buf.WriteByte('"')
	return nil
}

// writeStringRune writes one rune with the JCS short escapes, \u00XX for the
// other control characters, and every other rune verbatim.
func writeStringRune(buf *bytes.Buffer, r rune) {
	if escape, ok := shortEscapes[r]; ok {
		buf.WriteString(escape)
		return
	}
	if r < 0x20 {
		fmt.Fprintf(buf, `\u%04x`, r)
		return
	}
	buf.WriteRune(r)
}

var shortEscapes = map[rune]string{
	'"': `\"`, '\\': `\\`, '\b': `\b`, '\f': `\f`, '\n': `\n`, '\r': `\r`, '\t': `\t`,
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
	return slices.Compare(utf16Units(a), utf16Units(b))
}

func utf16Units(value string) []uint16 {
	units := make([]uint16, 0, len(value))
	for _, r := range value {
		if r <= 0xffff {
			units = append(units, uint16(r)) //nolint:gosec // r is proven <= 0xffff.
			continue
		}
		r -= 0x10000
		units = append(units, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff))) //nolint:gosec // UTF-16 surrogate ranges are bounded.
	}
	return units
}
