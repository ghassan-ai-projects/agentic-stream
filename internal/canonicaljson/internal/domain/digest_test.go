package domain

import (
	"math"
	"testing"
)

// testDomain is a digest domain private to these tests.
const testDomain Domain = "situation-runtime/test/v1\n"

func TestMarshalSortsObjectKeys(t *testing.T) {
	v := map[string]any{
		"z": 1,
		"a": 2,
		"m": 3,
	}
	got, err := Marshal(v)
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
	got, err := Marshal(v)
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
	d1, err := Digest(testDomain, v)
	if err != nil {
		t.Fatalf("Digest error: %v", err)
	}
	d2, err := Digest(testDomain, map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("Digest error: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("digests differ: %s vs %s", d1, d2)
	}
}

func TestMarshalFloat(t *testing.T) {
	got, err := Marshal(map[string]any{"v": 1.5})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"v":1.5}`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMarshalRejectsNonFinite(t *testing.T) {
	_, err := Marshal(map[string]any{"v": math.Inf(1)})
	if err == nil {
		t.Fatal("expected error for +Inf")
	}
	_, err = Marshal(map[string]any{"v": math.NaN()})
	if err == nil {
		t.Fatal("expected error for NaN")
	}
}

func TestMarshalRejectsNegativeZero(t *testing.T) {
	_, err := Marshal(map[string]any{"v": math.Copysign(0, -1)})
	if err == nil {
		t.Fatal("expected error for negative zero")
	}
}

func TestDigestHasDomainSeparation(t *testing.T) {
	got, err := Digest(testDomain, map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("Digest error: %v", err)
	}
	if len(got) != len("sha256:")+64 || got[:len("sha256:")] != "sha256:" {
		t.Fatalf("unexpected digest format: %s", got)
	}
	if !Verify(testDomain, map[string]any{"ok": true}, got) {
		t.Fatal("expected digest to verify")
	}
	if Verify(DomainSnapshot, map[string]any{"ok": true}, got) {
		t.Fatal("digest verified under the wrong domain")
	}
}

func TestDecodeDigestRequiresCanonicalPrefix(t *testing.T) {
	if _, err := DecodeDigest("0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected unprefixed digest to be rejected")
	}
	if got, err := DecodeDigest("sha256:0000000000000000000000000000000000000000000000000000000000000000"); err != nil || len(got) != 32 {
		t.Fatalf("expected prefixed digest to decode, got %x, %v", got, err)
	}
}
