package domain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/internal/domain"
)

func TestDeviceWireGoldenFramesValidate(t *testing.T) {
	t.Parallel()
	for _, messageType := range contractstest.MessageTypes {
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

func TestTheInvalidCorpusExercisesEveryMessageType(t *testing.T) {
	t.Parallel()
	covered := map[string]int{}
	for _, frame := range contractstest.InvalidFrames() {
		covered[frame.MessageType]++
	}
	for _, messageType := range contractstest.MessageTypes {
		if covered[messageType] == 0 {
			t.Errorf("no invalid conformance frame breaks a %s record", messageType)
		}
	}
}

func TestSchemaForMessageTypeKnowsOnlyTheFourDeviceRecords(t *testing.T) {
	t.Parallel()
	want := map[string]domain.SchemaName{
		"command": domain.SchemaDeviceCommand, "receipt": domain.SchemaDeviceReceipt,
		"result": domain.SchemaDeviceResult, "state": domain.SchemaDeviceState,
	}
	for messageType, schema := range want {
		if got, ok := domain.SchemaForMessageType(messageType); !ok || got != schema {
			t.Errorf("SchemaForMessageType(%q) = %q, %v; want %q", messageType, got, ok, schema)
		}
	}
	for _, unknown := range []string{"", "Command", "heartbeat", "device-command"} {
		if got, ok := domain.SchemaForMessageType(unknown); ok || got != "" {
			t.Errorf("SchemaForMessageType(%q) = %q, %v; want no schema", unknown, got, ok)
		}
	}
}

func TestValidConformanceFramesAreByteExactCanonicalJSON(t *testing.T) {
	t.Parallel()
	for _, messageType := range contractstest.MessageTypes {
		t.Run(messageType, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join("conformance", "v1", "valid", messageType+".json"))
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := canonicaljson.Marshal(contractstest.ValidFrame(messageType))
			if err != nil {
				t.Fatal(err)
			}
			if want := string(canonical) + "\n"; string(raw) != want {
				t.Fatalf("%s.json is not the canonical wire form:\n got %q\nwant %q", messageType, raw, want)
			}
		})
	}
}
