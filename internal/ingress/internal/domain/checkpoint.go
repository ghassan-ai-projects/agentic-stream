package domain

import (
	"encoding/json"
	"fmt"
)

// Checkpoint records how far a trace connector has read.
type Checkpoint struct {
	Version  int `json:"version"`
	LastLine int `json:"last_line"`
}

// CheckpointVersion is the only checkpoint layout in use.
const CheckpointVersion = 1

// EncodeCheckpoint serializes the position after lastLine lines.
func EncodeCheckpoint(lastLine int) ([]byte, error) {
	blob, err := json.Marshal(Checkpoint{Version: CheckpointVersion, LastLine: lastLine})
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint: %w", err)
	}
	return blob, nil
}

// DecodeCheckpoint reads the last line a stored checkpoint covers.
func DecodeCheckpoint(blob []byte) (int, error) {
	var checkpoint Checkpoint
	if err := json.Unmarshal(blob, &checkpoint); err != nil {
		return 0, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return checkpoint.LastLine, nil
}
