package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const maxDeviceFrameBytes = 64 * 1024

// EncodeDeviceRecord validates one supported device record and emits exactly
// one canonical NDJSON line. Raw framing remains the gateway's responsibility.
func EncodeDeviceRecord(document map[string]any) ([]byte, error) {
	if document == nil {
		return nil, fmt.Errorf("device record is required")
	}
	schema, err := deviceSchema(document)
	if err != nil {
		return nil, err
	}
	if err := contractsv1.Validate(schema, document); err != nil {
		return nil, fmt.Errorf("validate device record: %w", err)
	}
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode device record: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxDeviceFrameBytes {
		return nil, fmt.Errorf("device record exceeds %d bytes", maxDeviceFrameBytes)
	}
	return encoded, nil
}

// DecodeDeviceRecord decodes one supported NDJSON device record. It rejects
// trailing records, unknown message types, oversized input, and schema-invalid
// content before returning a document to the session.
func DecodeDeviceRecord(frame []byte) (map[string]any, error) {
	if len(frame) == 0 {
		return nil, fmt.Errorf("device frame is empty")
	}
	if len(frame) > maxDeviceFrameBytes {
		return nil, fmt.Errorf("device frame exceeds %d bytes", maxDeviceFrameBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode device frame: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("device frame must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("device frame contains trailing JSON")
		}
		return nil, fmt.Errorf("decode trailing device frame data: %w", err)
	}
	schema, err := deviceSchema(document)
	if err != nil {
		return nil, err
	}
	if err := contractsv1.Validate(schema, document); err != nil {
		return nil, fmt.Errorf("validate device frame: %w", err)
	}
	return document, nil
}

func deviceSchema(document map[string]any) (contractsv1.SchemaName, error) {
	messageType, ok := document["message_type"].(string)
	if !ok || messageType == "" {
		return "", fmt.Errorf("device message_type is required")
	}
	switch messageType {
	case "command":
		return contractsv1.SchemaDeviceCommand, nil
	case "receipt":
		return contractsv1.SchemaDeviceReceipt, nil
	case "result":
		return contractsv1.SchemaDeviceResult, nil
	case "state":
		return contractsv1.SchemaDeviceState, nil
	default:
		return "", fmt.Errorf("unsupported device message_type %q", messageType)
	}
}
