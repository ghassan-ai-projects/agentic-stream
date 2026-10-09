package domain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
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

func TestSharedAcceptVectorsCanonicalizeAndDigestAsRecorded(t *testing.T) {
	t.Parallel()
	vectors := readVectors(t)
	if len(vectors.Accept) != 16 {
		t.Fatalf("shared corpus has %d accept vectors, want 16", len(vectors.Accept))
	}
	for _, vector := range vectors.Accept {
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()
			requireMarshals(t, vector.Input, vector.Canonical)
			gotDigest, err := Digest(Domain(vector.Domain), vector.Input)
			if err != nil || gotDigest != vector.Digest {
				t.Fatalf("Digest = %q, %v; want %q", gotDigest, err, vector.Digest)
			}
		})
	}
}

func TestSharedNativeOnlyVectorsCanonicalizeGoDoubles(t *testing.T) {
	t.Parallel()
	nativeValues := map[string]float64{"double-integral-value": 1.0, "double-integral-negative": -2.0}
	vectors := readVectors(t)
	if len(vectors.NativeOnly) != len(nativeValues) {
		t.Fatalf("shared corpus has %d native-only vectors, the test knows %d", len(vectors.NativeOnly), len(nativeValues))
	}
	for _, vector := range vectors.NativeOnly {
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()
			value, known := nativeValues[vector.Name]
			if !known {
				t.Fatalf("native-only vector %q has no Go value in this test", vector.Name)
			}
			requireMarshals(t, map[string]any{"n": value}, vector.Canonical)
		})
	}
}

func TestSharedRejectVectorsAreRefused(t *testing.T) {
	t.Parallel()
	nativeValues := map[string]float64{
		"reject-nan":               math.NaN(),
		"reject-positive-infinity": math.Inf(1),
		"reject-negative-infinity": math.Inf(-1),
		"reject-negative-zero":     math.Copysign(0, -1),
	}
	vectors := readVectors(t)
	if len(vectors.Reject) != 8 {
		t.Fatalf("shared corpus has %d reject vectors, want 8", len(vectors.Reject))
	}
	for _, vector := range vectors.Reject {
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()
			if _, err := Marshal(rejectedInput(t, vector.Name, vector.InputForm, vector.Input, nativeValues)); err == nil {
				t.Fatal("the vector was accepted")
			}
		})
	}
}

func rejectedInput(t *testing.T, name, form, input string, native map[string]float64) any {
	t.Helper()
	switch form {
	case "native":
		value, known := native[name]
		if !known {
			t.Fatalf("native reject vector %q has no Go value in this test", name)
		}
		return value
	case "raw_json":
		return json.RawMessage(input)
	default:
		t.Fatalf("unknown input form %q", form)
		return nil
	}
}

func readVectors(t *testing.T) canonicalizationVectors {
	t.Helper()
	path := filepath.Join("..", "..", "..", "contractsv1", "internal", "domain", "testdata", "canonicalization-vectors.json")
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
