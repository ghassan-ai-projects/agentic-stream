package contractstest

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func TestFramesLoadForEveryMessageType(t *testing.T) {
	t.Parallel()
	for _, messageType := range MessageTypes {
		if len(ValidFrame(messageType)) == 0 {
			t.Fatalf("valid %s frame is empty", messageType)
		}
	}
	frames := InvalidFrames()
	if len(frames) == 0 {
		t.Fatal("no invalid frames")
	}
	for _, frame := range frames {
		if frame.Name == "" || frame.MessageType == "" || frame.Doc == nil {
			t.Fatalf("invalid frame %+v is incomplete", frame)
		}
	}
}

func TestMissingFramePanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("a missing frame did not panic")
		}
	}()
	ValidFrame("missing")
}

func TestAmbiguousKeyJSONDeclaresTheKeyTwiceDecoyFirst(t *testing.T) {
	t.Parallel()
	got := string(AmbiguousKeyJSON([]byte(`{"a":1}`), "k"))
	if want := `{"k":"decoy","a":1}`; got != want {
		t.Fatalf("AmbiguousKeyJSON = %s, want %s", got, want)
	}
	var lenient map[string]any
	if err := json.Unmarshal([]byte(`{"k":"decoy","k":"real"}`), &lenient); err != nil || lenient["k"] != "real" {
		t.Fatalf("a lenient reader keeps the later value: %v, %v", lenient, err)
	}
}

func TestRiskClassesListEveryClassInRisingOrder(t *testing.T) {
	t.Parallel()
	want := []contractsv1.RiskClass{contractsv1.RiskR0, contractsv1.RiskR1, contractsv1.RiskR2, contractsv1.RiskR3, contractsv1.RiskR4}
	got := RiskClasses()
	if len(got) != len(want) {
		t.Fatalf("RiskClasses = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != string(want[i]) {
			t.Fatalf("RiskClasses = %v, want %v", got, want)
		}
	}
}

func TestEveryInvalidFrameNamesTheMessageTypeItBreaks(t *testing.T) {
	t.Parallel()
	for _, frame := range InvalidFrames() {
		if !slices.Contains(MessageTypes, frame.MessageType) {
			t.Errorf("invalid frame %q names message type %q, which is not a device record", frame.Name, frame.MessageType)
		}
	}
}
