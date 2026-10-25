package plantwirefake

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// Birth is the contract's valid NBIRTH announcing every kind file of kindsDir
// with its kind digest, and one simulated board holding members.
func Birth(tb testing.TB, kindsDir, board string, members ...string) map[string]any {
	tb.Helper()
	birth := Fixture(tb, ".", "NBIRTH.json")
	paths, err := filepath.Glob(filepath.Join(kindsDir, "*.kind.json"))
	if err != nil || len(paths) == 0 {
		tb.Fatalf("no kind file in %s: %v", kindsDir, err)
	}
	kinds := make([]any, 0, len(paths))
	for _, path := range paths {
		kinds = append(kinds, announcedKind(tb, path))
	}
	birth["kinds"] = kinds
	birth["boards"] = []any{map[string]any{"board": board, "environment": "simulated", "members": members}}
	return birth
}

func announcedKind(tb testing.TB, path string) map[string]any {
	tb.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read the kind file: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var kind map[string]any
	if err := decoder.Decode(&kind); err != nil {
		tb.Fatalf("decode the kind file %s: %v", path, err)
	}
	kind["kind_digest"] = kindDigest(tb, kind)
	return kind
}

func kindDigest(tb testing.TB, kind map[string]any) string {
	tb.Helper()
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(kind); err != nil {
		tb.Fatalf("encode the kind %v: %v", kind["kind"], err)
	}
	sum := sha256.Sum256(bytes.TrimSuffix(canonical.Bytes(), []byte("\n")))
	return kernel.EncodeDigest(sum[:])
}
