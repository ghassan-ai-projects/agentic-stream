package contractstest

import "testing"

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
