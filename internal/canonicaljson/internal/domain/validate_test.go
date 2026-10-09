package domain

import (
	"encoding/json"
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
		{name: "upper case hex escape", json: `"\u00E9"`},
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

func TestMarshalAdmitsOnlyUnambiguousRawJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "duplicate key", raw: `{"a":1,"a":2}`, want: `duplicate object key "a"`},
		{name: "escaped-equivalent duplicate key", raw: `{"a":1,"\u0061":2}`, want: `duplicate object key "a"`},
		{name: "duplicate key in a nested object", raw: `{"o":{"k":1,"k":2}}`, want: "duplicate object key"},
		{name: "duplicate key inside an array element", raw: `[{"k":1,"k":2}]`, want: "duplicate object key"},
		{name: "same key in different objects is fine to repeat", raw: `[{"k":1},{"k":2}]`},
		{name: "two values", raw: `{"a":1} {"b":2}`, want: "multiple JSON values"},
		{name: "trailing garbage", raw: `{"a":1} x`, want: "trailing"},
		{name: "unterminated object", raw: `{"a":1`, want: "read JSON"},
		{name: "unterminated array", raw: `[1,2`, want: "read JSON"},
		{name: "missing colon", raw: `{"a" 1}`, want: "read JSON"},
		{name: "non-string key", raw: `{1:2}`, want: "read JSON"},
		{name: "trailing comma", raw: `[1,]`, want: "read JSON"},
		{name: "lone closing bracket", raw: `]`, want: "read JSON"},
		{name: "empty input", raw: ``, want: "read JSON"},
		{name: "only whitespace", raw: ` `, want: "read JSON"},
		{name: "escaped surrogate pair", raw: `{"k":"\ud83d\ude00"}`},
		{name: "lone escaped high surrogate", raw: `{"k":"\ud800"}`, want: "lone high surrogate"},
		{name: "lone escaped low surrogate", raw: `{"k":"\udc00"}`, want: "lone low surrogate"},
		{name: "surrogate in a key", raw: `{"\ud800":1}`, want: "lone high surrogate"},
		{name: "whitespace around values", raw: " \n\t[ 1 , { \"a\" : null } ] \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Marshal(json.RawMessage(tt.raw))
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Marshal(%s) = %v", tt.raw, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Marshal(%s) = %v, want an error containing %q", tt.raw, err, tt.want)
			}
		})
	}
}

func TestMarshalCanonicalizesEscapedRawStrings(t *testing.T) {
	t.Parallel()

	requireMarshals(t, json.RawMessage(`{"k":"\ud83d\ude00"}`), "{\"k\":\"\U0001f600\"}")
	requireMarshals(t, json.RawMessage(`{"\u0041":"\u00e9\n\/"}`), "{\"A\":\"\u00e9\\n/\"}")
}
