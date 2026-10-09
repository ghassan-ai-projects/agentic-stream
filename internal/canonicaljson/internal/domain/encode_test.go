package domain

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func marshalString(v any) (string, error) {
	b, err := Marshal(v)
	return string(b), err
}

func requireMarshals(t *testing.T, value any, want string) {
	t.Helper()
	got, err := marshalString(value)
	if err != nil || got != want {
		t.Fatalf("Marshal(%#v) = %q, %v; want %q", value, got, err, want)
	}
}

func requireMarshalFails(t *testing.T, value any, want string) {
	t.Helper()
	got, err := marshalString(value)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Marshal(%#v) = %q, %v; want an error containing %q", value, got, err, want)
	}
}

func TestMarshalSortsObjectKeysByUTF16CodeUnits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		keys []string
	}{
		{"RFC 8785 section 3.2.3 sample", []string{"\r", "1", "\u0080", "ö", "€", "\U0001f600", "דּ"}},
		{"an astral key sorts before U+E000: its first code unit is a high surrogate", []string{"\U0001f600", ""}},
		{"upper case before lower case", []string{"A", "a", "b"}},
		{"a prefix sorts before its extension", []string{"", "a", "ab"}},
		{"digits compare as text", []string{"1", "10", "9"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			object := make(map[string]any, len(tt.keys))
			var want []string
			for i, key := range tt.keys {
				object[key] = i
				want = append(want, mustMarshal(t, key)+":"+mustMarshal(t, i))
			}
			requireMarshals(t, object, "{"+strings.Join(want, ",")+"}")
		})
	}
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	got, err := marshalString(v)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMarshalEncodesEveryJSONShape(t *testing.T) {
	t.Parallel()
	type member struct {
		Name  string `json:"name"`
		Count int    `json:"count,omitempty"`
		Skip  string `json:"-"`
	}
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"null", nil, `null`},
		{"true", true, `true`},
		{"false", false, `false`},
		{"empty object", map[string]any{}, `{}`},
		{"empty array", []any{}, `[]`},
		{"array keeps its order", []any{3, 1, 2}, `[3,1,2]`},
		{"nested objects are sorted at every depth", map[string]any{"b": []any{map[string]any{"y": 1, "x": 2}}, "a": true}, `{"a":true,"b":[{"x":2,"y":1}]}`},
		{"null member", map[string]any{"n": nil}, `{"n":null}`},
		{"struct goes through its json tags", member{Name: "m", Skip: "x"}, `{"name":"m"}`},
		{"struct pointer", &member{Name: "m", Count: 2}, `{"count":2,"name":"m"}`},
		{"typed slice", []string{"b", "a"}, `["b","a"]`},
		{"typed map", map[string]int{"z": 1, "a": 2}, `{"a":2,"z":1}`},
		{"raw message is canonicalized", json.RawMessage(` { "b" : 1 , "a" : [ true ] } `), `{"a":[true],"b":1}`},
		{"raw message inside a map", map[string]any{"k": json.RawMessage(`{"y":1,"x":2}`)}, `{"k":{"x":2,"y":1}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshals(t, tt.value, tt.want)
		})
	}
}

func TestMarshalRefusesWhatCannotBeCanonicalized(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"object key with invalid UTF-8", map[string]any{"\xc3(": 1}, "invalid UTF-8 object key"},
		{"string with invalid UTF-8", "\xc3(", "not well-formed Unicode"},
		{"nested string with invalid UTF-8", map[string]any{"k": []any{"\xff"}}, "not well-formed Unicode"},
		{"unsupported kind", make(chan int), "fallback marshal"},
		{"func", func() {}, "fallback marshal"},
		{"struct holding NaN", struct{ N float64 }{N: math.NaN()}, "fallback marshal"},
		{"malformed raw message", json.RawMessage(`{`), "invalid RawMessage"},
		{"empty raw message", json.RawMessage(``), "invalid RawMessage"},
		{"non-finite inside an array", []any{1, math.Inf(1)}, "non-finite float"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireMarshalFails(t, tt.value, tt.want)
		})
	}
}
