package domain

import (
	"strings"
	"testing"
)

func TestCheckpointRoundTripsTheLastLine(t *testing.T) {
	t.Parallel()
	blob, err := EncodeCheckpoint(42)
	if err != nil || string(blob) != `{"version":1,"last_line":42}` {
		t.Fatalf("blob = %s, err = %v", blob, err)
	}
	if line, err := DecodeCheckpoint(blob); err != nil || line != 42 {
		t.Fatalf("line = %d, err = %v", line, err)
	}
}

func TestDecodeCheckpointRefusesCorruption(t *testing.T) {
	t.Parallel()
	if _, err := DecodeCheckpoint([]byte("{")); err == nil || !strings.Contains(err.Error(), "unmarshal checkpoint") {
		t.Fatalf("err = %v, want an unmarshal failure", err)
	}
}
