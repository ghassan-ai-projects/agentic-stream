// Package contractstest is test support for the contractsv1 module: it loads
// the committed device-wire conformance frames under
// internal/contractsv1/internal/domain/conformance/v1/, the same files the
// Streams Simulator, the gateway and the firmware test against. Import it from
// tests only.
package contractstest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// MessageTypes are the four device-wire record types, in wire order.
var MessageTypes = []string{"command", "receipt", "result", "state"}

// InvalidFrame is one negative case: a frame that must be rejected, and the
// message type whose schema it violates (the prefix of its file name).
type InvalidFrame struct {
	Name, MessageType string
	Doc               map[string]any
}

// ValidFrame loads the canonical valid frame for one message type, panicking
// on a fixture defect.
func ValidFrame(messageType string) map[string]any {
	doc, err := loadFrame(filepath.Join(conformanceDir(), "valid", messageType+".json"))
	if err != nil {
		panic(err)
	}
	return doc
}

// InvalidFrames loads the negative corpus in name order.
func InvalidFrames() []InvalidFrame {
	dir := filepath.Join(conformanceDir(), "invalid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		panic(fmt.Errorf("read conformance invalid dir: %w", err))
	}
	frames := make([]InvalidFrame, 0, len(entries))
	for _, entry := range entries {
		frames = append(frames, invalidFrame(dir, entry.Name()))
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].Name < frames[j].Name })
	return frames
}

func invalidFrame(dir, fileName string) InvalidFrame {
	name := strings.TrimSuffix(fileName, ".json")
	doc, err := loadFrame(filepath.Join(dir, fileName))
	if err != nil {
		panic(err)
	}
	messageType, _, _ := strings.Cut(name, "-")
	return InvalidFrame{Name: name, MessageType: messageType, Doc: doc}
}

func conformanceDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "internal", "domain", "conformance", "v1")
}

func loadFrame(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // committed fixture paths
	if err != nil {
		return nil, fmt.Errorf("read conformance frame %s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode conformance frame %s: %w", path, err)
	}
	return doc, nil
}
