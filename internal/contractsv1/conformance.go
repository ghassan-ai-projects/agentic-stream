package contractsv1

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Device wire conformance frames. The committed JSON under conformance/v1/ is the
// single source of truth — example device records the other program repos (the
// Streams Simulator device emulator, the edge gateway, firmware) copy and test
// against. This file is machinery that LOADS that data via go:embed; the frames
// are never Go literals (AGENTS.md: domain data is JSON, loaded by machinery).
// See conformance/README.md.

//go:embed conformance/v1/valid/*.json conformance/v1/invalid/*.json
var conformanceFiles embed.FS

// ConformanceValidMessageTypes lists the device message types, in wire order.
// These are the four contract record types (defined by the schemas), not
// domain-flavored values.
func ConformanceValidMessageTypes() []string {
	return []string{"command", "receipt", "result", "state"}
}

// ConformanceValidFrame loads the canonical valid frame for one device message
// type from the committed conformance data.
func ConformanceValidFrame(messageType string) map[string]any {
	doc, err := loadConformanceFrame("conformance/v1/valid/" + messageType + ".json")
	if err != nil {
		panic(err)
	}
	return doc
}

// InvalidFrame is one negative conformance case loaded from data: a frame that
// MUST be rejected, with the schema it is meant to violate (from the filename).
type InvalidFrame struct {
	Name   string
	Schema SchemaName
	Doc    map[string]any
}

// ConformanceInvalidFrames loads the negative corpus from the committed data.
func ConformanceInvalidFrames() []InvalidFrame {
	entries, err := conformanceFiles.ReadDir("conformance/v1/invalid")
	if err != nil {
		panic(fmt.Errorf("read conformance invalid dir: %w", err))
	}
	frames := make([]InvalidFrame, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".json")
		doc, err := loadConformanceFrame("conformance/v1/invalid/" + entry.Name())
		if err != nil {
			panic(err)
		}
		schema, ok := schemaForInvalidName(name)
		if !ok {
			panic(fmt.Errorf("contractsv1: cannot map invalid conformance frame %q to a schema", name))
		}
		frames = append(frames, InvalidFrame{Name: name, Schema: schema, Doc: doc})
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i].Name < frames[j].Name })
	return frames
}

func loadConformanceFrame(path string) (map[string]any, error) {
	data, err := conformanceFiles.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read conformance frame %s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode conformance frame %s: %w", path, err)
	}
	return doc, nil
}

// schemaForInvalidName maps an invalid fixture's filename prefix to the schema it
// violates (command-* → device-command, and so on).
func schemaForInvalidName(name string) (SchemaName, bool) {
	switch {
	case strings.HasPrefix(name, "command-"):
		return SchemaDeviceCommand, true
	case strings.HasPrefix(name, "receipt-"):
		return SchemaDeviceReceipt, true
	case strings.HasPrefix(name, "result-"):
		return SchemaDeviceResult, true
	case strings.HasPrefix(name, "state-"):
		return SchemaDeviceState, true
	default:
		return "", false
	}
}

// SchemaForMessageType maps a device message_type to its schema name.
func SchemaForMessageType(messageType string) (SchemaName, bool) {
	switch messageType {
	case "command":
		return SchemaDeviceCommand, true
	case "receipt":
		return SchemaDeviceReceipt, true
	case "result":
		return SchemaDeviceResult, true
	case "state":
		return SchemaDeviceState, true
	default:
		return "", false
	}
}
