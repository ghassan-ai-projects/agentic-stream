package contractsv1_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// The committed conformance fixtures under conformance/v1/ are the cross-repo
// contract surface: the Streams Simulator device emulator, the edge gateway, and
// the firmware copy these bytes. They are DATA (loaded via go:embed), and these
// tests pin them against the schemas so a fixture can never silently drift from
// the contract Agentic Stream enforces — every valid frame must validate, and
// every invalid frame must fail closed.

func TestConformanceValidFramesValidate(t *testing.T) {
	for _, messageType := range contractsv1.ConformanceValidMessageTypes() {
		messageType := messageType
		t.Run(messageType, func(t *testing.T) {
			schema, ok := contractsv1.SchemaForMessageType(messageType)
			if !ok {
				t.Fatalf("no schema for message_type %q", messageType)
			}
			if err := contractsv1.Validate(schema, contractsv1.ConformanceValidFrame(messageType)); err != nil {
				t.Fatalf("valid %s fixture must validate: %v", messageType, err)
			}
		})
	}
}

func TestConformanceInvalidFramesFailClosed(t *testing.T) {
	frames := contractsv1.ConformanceInvalidFrames()
	if len(frames) == 0 {
		t.Fatal("expected a non-empty invalid conformance corpus")
	}
	seen := map[string]bool{}
	for _, frame := range frames {
		frame := frame
		if seen[frame.Name] {
			t.Fatalf("duplicate invalid frame name %q", frame.Name)
		}
		seen[frame.Name] = true
		t.Run(frame.Name, func(t *testing.T) {
			if err := contractsv1.Validate(frame.Schema, frame.Doc); err == nil {
				t.Fatalf("invalid fixture %q must fail closed, but validated", frame.Name)
			}
		})
	}
}
