package domain

import (
	"encoding/json"
	"fmt"
)

type Checkpoint struct {
	Version  int   `json:"version"`
	LastLine int   `json:"last_line"`
	Offset   int64 `json:"offset,omitempty"`
}

const CheckpointVersion = 1

func EncodeCheckpoint(lastLine int) ([]byte, error) {
	return EncodePosition(Checkpoint{LastLine: lastLine})
}

func EncodePosition(position Checkpoint) ([]byte, error) {
	position.Version = CheckpointVersion
	blob, err := json.Marshal(position)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint: %w", err)
	}
	return blob, nil
}

func DecodeCheckpoint(blob []byte) (int, error) {
	position, err := DecodePosition(blob)
	return position.LastLine, err
}

func DecodePosition(blob []byte) (Checkpoint, error) {
	var checkpoint Checkpoint
	if err := json.Unmarshal(blob, &checkpoint); err != nil {
		return Checkpoint{}, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return checkpoint, nil
}
