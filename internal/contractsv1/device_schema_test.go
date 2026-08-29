package contractsv1_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Device wire schemas for the serial effector boundary (Real-World Sensor
// HIL-0, Phase 03 Task 3.1). The contract is the single source of truth shared
// with the gateway/emulator/firmware repos. These tests pin the golden frames
// and prove the codec fails closed on malformed, oversized, wrong-version,
// unknown-type, and unknown-field frames — an invalid frame must never validate
// into a usable record.
//
// The frames themselves live in conformance.go (contractsv1.ConformanceValidFrame
// / ConformanceInvalidFrames) so the schema pinning here, the committed fixtures
// under conformance/v1/, and the copies the other repos test against are all one
// source. See conformance_test.go and conformance/README.md.

func TestDeviceWireGoldenFramesValidate(t *testing.T) {
	t.Parallel()
	for _, messageType := range contractsv1.ConformanceValidMessageTypes() {
		messageType := messageType
		t.Run(messageType, func(t *testing.T) {
			t.Parallel()
			schema, ok := contractsv1.SchemaForMessageType(messageType)
			if !ok {
				t.Fatalf("no schema for message_type %q", messageType)
			}
			if err := contractsv1.Validate(schema, contractsv1.ConformanceValidFrame(messageType)); err != nil {
				t.Fatalf("golden %s frame must validate: %v", messageType, err)
			}
		})
	}
}

func TestDeviceWireFramesFailClosed(t *testing.T) {
	t.Parallel()
	frames := contractsv1.ConformanceInvalidFrames()
	if len(frames) == 0 {
		t.Fatal("expected a non-empty invalid conformance corpus")
	}
	for _, frame := range frames {
		frame := frame
		t.Run(frame.Name, func(t *testing.T) {
			t.Parallel()
			if err := contractsv1.Validate(frame.Schema, frame.Doc); err == nil {
				t.Fatalf("mutated %s frame must fail closed, but validated", frame.Name)
			}
		})
	}
}
