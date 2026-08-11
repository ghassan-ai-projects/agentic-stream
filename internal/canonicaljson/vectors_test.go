package canonicaljson_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

type canonicalizationVectors struct {
	NativeOnly []struct {
		Name      string `json:"name"`
		Domain    string `json:"domain"`
		Canonical string `json:"canonical"`
	} `json:"native_only"`
	Accept []struct {
		Name      string `json:"name"`
		Domain    string `json:"domain"`
		Input     any    `json:"input"`
		Canonical string `json:"canonical"`
		Digest    string `json:"digest"`
	} `json:"accept"`
	Reject []struct {
		Name      string `json:"name"`
		InputForm string `json:"input_form"`
		Input     string `json:"input"`
	} `json:"reject"`
}

func TestSharedCanonicalizationVectors(t *testing.T) {
	vectors := readVectors(t)
	if len(vectors.Accept) != 16 {
		t.Fatalf("expected 16 accept vectors, got %d", len(vectors.Accept))
	}
	if len(vectors.Reject) != 8 {
		t.Fatalf("expected 8 reject vectors, got %d", len(vectors.Reject))
	}

	for _, vector := range vectors.Accept {
		vector := vector
		t.Run(vector.Name, func(t *testing.T) {
			gotCanonical, err := canonicaljson.MarshalString(vector.Input)
			if err != nil {
				t.Fatalf("MarshalString failed: %v", err)
			}
			if gotCanonical != vector.Canonical {
				t.Fatalf("canonical = %q, want %q", gotCanonical, vector.Canonical)
			}
			gotDigest, err := canonicaljson.Digest(canonicaljson.Domain(vector.Domain), vector.Input)
			if err != nil {
				t.Fatalf("Digest failed: %v", err)
			}
			if gotDigest != vector.Digest {
				t.Fatalf("digest = %q, want %q", gotDigest, vector.Digest)
			}
		})
	}
}

func TestNativeOnlyDoubleVectors(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		canonical string
	}{
		{name: "double-integral-value", value: 1.0, canonical: `{"n":1}`},
		{name: "double-integral-negative", value: -2.0, canonical: `{"n":-2}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := canonicaljson.MarshalString(map[string]any{"n": test.value})
			if err != nil {
				t.Fatalf("MarshalString failed: %v", err)
			}
			if got != test.canonical {
				t.Fatalf("canonical = %q, want %q", got, test.canonical)
			}
		})
	}
}

func TestSharedRejectVectors(t *testing.T) {
	vectors := readVectors(t)
	for _, vector := range vectors.Reject {
		vector := vector
		t.Run(vector.Name, func(t *testing.T) {
			var value any
			switch vector.InputForm {
			case "native":
				switch vector.Name {
				case "reject-nan":
					value = math.NaN()
				case "reject-positive-infinity":
					value = math.Inf(1)
				case "reject-negative-infinity":
					value = math.Inf(-1)
				case "reject-negative-zero":
					value = math.Copysign(0, -1)
				default:
					t.Fatalf("unknown native vector %q", vector.Name)
				}
			case "raw_json":
				value = json.RawMessage(vector.Input)
			default:
				t.Fatalf("unknown input form %q", vector.InputForm)
			}
			if _, err := canonicaljson.Marshal(value); err == nil {
				t.Fatal("expected vector to be rejected")
			}
		})
	}
}

func TestNativeIntegerOutsideExactRangeIsRejected(t *testing.T) {
	if _, err := canonicaljson.Marshal(int64(9007199254740993)); err == nil {
		t.Fatal("expected unsafe native integer to be rejected")
	}
}

func TestRawUnsafeIntegerSpellingsAreRejected(t *testing.T) {
	for _, raw := range []string{
		`{"n":9007199254740992}`,
		`{"n":9007199254740993.0}`,
		`{"n":9007199254740993e0}`,
	} {
		if _, err := canonicaljson.Marshal(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected unsafe integer spelling %s to be rejected", raw)
		}
	}
}

func TestRawValidEscapedSurrogatePairIsAccepted(t *testing.T) {
	got, err := canonicaljson.MarshalString(json.RawMessage(`{"k":"\ud83d\ude00"}`))
	if err != nil {
		t.Fatalf("expected valid surrogate pair to be accepted: %v", err)
	}
	if got != `{"k":"😀"}` {
		t.Fatalf("canonical = %q, want %q", got, `{"k":"😀"}`)
	}
}

func TestRawEscapedEquivalentDuplicateKeysAreRejected(t *testing.T) {
	if _, err := canonicaljson.Marshal(json.RawMessage(`{"a":1,"\u0061":2}`)); err == nil {
		t.Fatal("expected escaped-equivalent duplicate keys to be rejected")
	}
}

func TestExactlyRepresentableDoubleAtSafeBoundaryIsAccepted(t *testing.T) {
	got, err := canonicaljson.MarshalString(map[string]any{"n": float64(1 << 53)})
	if err != nil {
		t.Fatalf("expected exactly representable double to be accepted: %v", err)
	}
	if got != `{"n":9007199254740992}` {
		t.Fatalf("canonical = %q, want %q", got, `{"n":9007199254740992}`)
	}
}

func TestNativeInvalidUnicodeIsRejected(t *testing.T) {
	if _, err := canonicaljson.Marshal(string([]byte{'\xc3', '('})); err == nil {
		t.Fatal("expected invalid UTF-8 to be rejected")
	}
}

func readVectors(t *testing.T) canonicalizationVectors {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "..", "contractsv1", "testdata", "canonicalization-vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var vectors canonicalizationVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("decode vectors: %v", err)
	}
	return vectors
}
