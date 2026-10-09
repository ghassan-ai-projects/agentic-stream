package domain

import "testing"

func TestMarshalEscapesStringsAsRFC8785Requires(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain ASCII", "plain", `"plain"`},
		{"empty", "", `""`},
		{"quote", `a"b`, `"a\"b"`},
		{"backslash", `a\b`, `"a\\b"`},
		{"backspace", "\b", `"\b"`},
		{"form feed", "\f", `"\f"`},
		{"line feed", "\n", `"\n"`},
		{"carriage return", "\r", `"\r"`},
		{"tab", "\t", `"\t"`},
		{"NUL", "\x00", `"\u0000"`},
		{"other control characters use lower case hex", "\x01\x0b\x1f", `"\u0001\u000b\u001f"`},
		{"solidus is not escaped", "a/b", `"a/b"`},
		{"DEL is not escaped", "\x7f", "\"\x7f\""},
		{"first character after the control range is verbatim", " ", `" "`},
		{"non-ASCII is verbatim", "café €", "\"café €\""},
		{"line separator is verbatim", "  ", "\"  \""},
		{"astral character is verbatim", "\U0001f600", "\"\U0001f600\""},
		{"U+FFFF is verbatim", "￿", "\"￿\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshals(t, tt.value, tt.want)
		})
	}
}

func TestMarshalEscapesObjectKeysLikeStrings(t *testing.T) {
	t.Parallel()
	requireMarshals(t, map[string]any{"a\"b\n": 1}, `{"a\"b\n":1}`)
}

func TestCompareUTF16OrdersByCodeUnitNotByCodePoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "ab", "ab", 0},
		{"smaller first unit", "a", "b", -1},
		{"prefix is smaller", "a", "ab", -1},
		{"empty is smaller", "", "a", -1},
		{"astral below U+E000", "\U0001f600", "", -1},
		{"U+E000 above astral", "", "\U0001f600", 1},
		{"astral above U+D7FF", "\U00010000", "퟿", 1},
		{"astral compares by low surrogate after equal high", "\U0001f600", "\U0001f601", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := compareUTF16(tt.a, tt.b); got != tt.want {
				t.Fatalf("compareUTF16(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestUTF16UnitsSplitsAstralCharactersIntoSurrogatePairs(t *testing.T) {
	t.Parallel()
	got := utf16Units("a\U0001f600€")
	want := []uint16{0x61, 0xd83d, 0xde00, 0x20ac}
	if len(got) != len(want) {
		t.Fatalf("utf16Units = %x, want %x", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("utf16Units = %x, want %x", got, want)
		}
	}
}

func TestMarshalRefusesStringsThatAreNotWellFormedUnicode(t *testing.T) {
	t.Parallel()
	for name, value := range map[string]string{
		"truncated sequence":     "\xc3",
		"overlong encoding":      "\xc0\x80",
		"encoded high surrogate": "\xed\xa0\x80",
		"encoded low surrogate":  "\xed\xb0\x80",
		"out of range":           "\xf4\x90\x80\x80",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requireMarshalFails(t, value, "not well-formed Unicode")
		})
	}
}
