package domain_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/internal/domain"
)

// Device wire schemas for the serial effector boundary (Real-World Sensor
// HIL-0, Phase 03 Task 3.1). The contract is the single source of truth shared
// with the gateway/emulator/firmware repos. These tests pin the golden frames
// and prove the codec fails closed on malformed, oversized, wrong-version,
// unknown-type, and unknown-field frames — an invalid frame must never validate
// into a usable record.
//
// The frames themselves are loaded by contractstest from committed fixtures
// under conformance/v1/, so the schema pinning here and the copies the other
// repos test against are all one source. See conformance/README.md.

func TestDeviceWireGoldenFramesValidate(t *testing.T) {
	t.Parallel()
	for _, messageType := range contractstest.MessageTypes {
		messageType := messageType
		t.Run(messageType, func(t *testing.T) {
			t.Parallel()
			schema, ok := domain.SchemaForMessageType(messageType)
			if !ok {
				t.Fatalf("no schema for message_type %q", messageType)
			}
			if err := domain.Validate(schema, contractstest.ValidFrame(messageType)); err != nil {
				t.Fatalf("golden %s frame must validate: %v", messageType, err)
			}
		})
	}
}

func TestDeviceWireFramesFailClosed(t *testing.T) {
	t.Parallel()
	frames := contractstest.InvalidFrames()
	if len(frames) == 0 {
		t.Fatal("expected a non-empty invalid conformance corpus")
	}
	for _, frame := range frames {
		frame := frame
		t.Run(frame.Name, func(t *testing.T) {
			t.Parallel()
			schema, ok := domain.SchemaForMessageType(frame.MessageType)
			if !ok {
				t.Fatalf("cannot map invalid frame %q to a schema", frame.Name)
			}
			if err := domain.Validate(schema, frame.Doc); err == nil {
				t.Fatalf("mutated %s frame must fail closed, but validated", frame.Name)
			}
		})
	}
}
