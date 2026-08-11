package canonicaljson_test

import (
	"math"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestMarshalSortsObjectKeys(t *testing.T) {
	v := map[string]any{
		"z": 1,
		"a": 2,
		"m": 3,
	}
	got, err := canonicaljson.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"a":2,"m":3,"z":1}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMarshalNested(t *testing.T) {
	v := map[string]any{
		"b": []any{map[string]any{"y": 1, "x": 2}},
		"a": true,
	}
	got, err := canonicaljson.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"a":true,"b":[{"x":2,"y":1}]}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestDigestStable(t *testing.T) {
	v := map[string]any{"b": 2, "a": 1}
	d1, err := canonicaljson.Digest(v)
	if err != nil {
		t.Fatalf("Digest error: %v", err)
	}
	d2, err := canonicaljson.Digest(map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("Digest error: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("digests differ: %s vs %s", d1, d2)
	}
}

func TestMarshalFloat(t *testing.T) {
	got, err := canonicaljson.Marshal(map[string]any{"v": 1.5})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"v":1.5}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMarshalRejectsNonFinite(t *testing.T) {
	_, err := canonicaljson.Marshal(map[string]any{"v": math.Inf(1)})
	if err == nil {
		t.Fatal("expected error for +Inf")
	}
	_, err = canonicaljson.Marshal(map[string]any{"v": math.NaN()})
	if err == nil {
		t.Fatal("expected error for NaN")
	}
}
