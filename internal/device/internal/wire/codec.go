package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// MaxFrameBytes is the largest device record, newline included.
const MaxFrameBytes = 64 * 1024

// Encode validates one supported device record and emits exactly
// one canonical NDJSON line. Raw framing remains the gateway's responsibility.
func Encode(document map[string]any) ([]byte, error) {
	if document == nil {
		return nil, fmt.Errorf("device record is required")
	}
	schema, err := schemaFor(document)
	if err != nil {
		return nil, err
	}
	if err := contractsv1.Validate(schema, document); err != nil {
		return nil, fmt.Errorf("validate device record: %w", err)
	}
	return encodeFrame(document)
}

func encodeFrame(document map[string]any) ([]byte, error) {
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode device record: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > MaxFrameBytes {
		return nil, fmt.Errorf("device record exceeds %d bytes", MaxFrameBytes)
	}
	return encoded, nil
}

// Decode decodes one supported NDJSON device record. It rejects
// trailing records, unknown message types, oversized input, and schema-invalid
// content before returning a document to the session.
func Decode(frame []byte) (map[string]any, error) {
	if len(frame) == 0 {
		return nil, fmt.Errorf("device frame is empty")
	}
	if len(frame) > MaxFrameBytes {
		return nil, fmt.Errorf("device frame exceeds %d bytes", MaxFrameBytes)
	}
	return decodeFrame(frame)
}

func decodeFrame(frame []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(frame))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode device frame: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("device frame must be a JSON object")
	}
	return validateDecodedFrame(decoder, document)
}

func validateDecodedFrame(decoder *json.Decoder, document map[string]any) (map[string]any, error) {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("device frame contains trailing JSON")
		}
		return nil, fmt.Errorf("decode trailing device frame data: %w", err)
	}
	schema, err := schemaFor(document)
	if err != nil {
		return nil, err
	}
	if err := contractsv1.Validate(schema, document); err != nil {
		return nil, fmt.Errorf("validate device frame: %w", err)
	}
	return document, nil
}

func schemaFor(document map[string]any) (contractsv1.SchemaName, error) {
	messageType, ok := document["message_type"].(string)
	if !ok || messageType == "" {
		return "", fmt.Errorf("device message_type is required")
	}
	schema, ok := contractsv1.SchemaForMessageType(messageType)
	if !ok {
		return "", fmt.Errorf("unsupported device message_type %q", messageType)
	}
	return schema, nil
}
