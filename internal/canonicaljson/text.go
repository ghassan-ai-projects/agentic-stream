package canonicaljson

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

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
			units = append(units, uint16(r)) //nolint:gosec // r is proven <= 0xffff.
			continue
		}
		r -= 0x10000
		units = append(units, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff))) //nolint:gosec // UTF-16 surrogate ranges are bounded.
	}
	return units
}
