package wire

import (
	"encoding/json"
	"fmt"
)

func Encode(record map[string]any) ([]byte, error) {
	line, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotSent, err)
	}
	in, err := Decode(line)
	if err == nil {
		err = in.Invalid
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotSent, err)
	}
	return line, nil
}
