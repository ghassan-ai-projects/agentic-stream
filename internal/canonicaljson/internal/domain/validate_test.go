package domain

import (
	"strings"
	"testing"
)

func TestValidateStringsRejectsEveryMalformedString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "simple escapes", json: `{"a":"\" \\ \/ \b \f \n \r \t"}`},
		{name: "unicode escape", json: `"é"`},
		{name: "surrogate pair", json: `"😀"`},
		{name: "raw multibyte UTF-8", json: "\"café\""},
		{name: "unknown escape", json: `"\x"`, want: "invalid JSON escape"},
		{name: "escape at end", json: `"\`, want: "unterminated JSON escape"},
		{name: "short unicode escape", json: `"\u12"`, want: "short Unicode escape"},
		{name: "non-hex unicode escape", json: `"\u12zz"`, want: "invalid Unicode escape"},
		{name: "lone low surrogate", json: `"\ude00"`, want: "lone low surrogate"},
		{name: "lone high surrogate", json: `"\ud83d"`, want: "lone high surrogate"},
		{name: "high surrogate then text", json: `"\ud83dabcdefg"`, want: "lone high surrogate"},
		{name: "high surrogate then non-low escape", json: `"\ud83d\u0041"`, want: "invalid surrogate pair"},
		{name: "raw control character", json: "\"a\x01b\"", want: "unescaped control character"},
		{name: "invalid UTF-8", json: "\"a\xffb\"", want: "invalid UTF-8"},
		{name: "unterminated string", json: `"abc`, want: "unterminated JSON string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateStrings([]byte(tt.json))
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateStrings(%q) = %v", tt.json, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateStrings(%q) = %v, want %q", tt.json, err, tt.want)
			}
		})
	}
}

func TestScaleDigits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		digits      string
		places      int
		want        string
		wantInteger bool
	}{
		{digits: "12", places: -2, want: "1200", wantInteger: true},
		{digits: "12", places: 0, want: "12", wantInteger: true},
		{digits: "1200", places: 2, want: "12", wantInteger: true},
		{digits: "1250", places: 2, want: "12", wantInteger: false},
		{digits: "000", places: 5, want: "0", wantInteger: true},
		{digits: "001", places: 5, want: "0", wantInteger: false},
	}
	for _, tt := range tests {
		got, ok := scaleDigits(tt.digits, tt.places)
		if got != tt.want || ok != tt.wantInteger {
			t.Errorf("scaleDigits(%q, %d) = %q, %v; want %q, %v", tt.digits, tt.places, got, ok, tt.want, tt.wantInteger)
		}
	}
}
