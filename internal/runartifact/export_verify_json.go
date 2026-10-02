package runartifact

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func verifyJSONFiles(dir string) error {
	for _, name := range exportedJSON {
		data, err := readArtifactFile(dir, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := verifyJSON(data); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
	}
	return nil
}

func verifyJSON(data []byte) error {
	var value any
	if err := json.Unmarshal(bytesTrimSpace(data), &value); err != nil {
		return fmt.Errorf("decode stored JSON: %w", err)
	}
	canonical, err := canonicaljson.Marshal(value)
	if err != nil {
		return fmt.Errorf("canonicalize stored JSON: %w", err)
	}
	if string(canonical) != string(bytesTrimSpace(data)) {
		return fmt.Errorf("JSON is not canonical")
	}
	return nil
}

func verifyJSONL(data []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		if err := verifyJSONLRecord(scanner.Bytes()); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan JSONL: %w", err)
	}
	return nil
}

func verifyJSONLRecord(data []byte) error {
	if len(bytesTrimSpace(data)) == 0 {
		return fmt.Errorf("blank JSONL record")
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode JSONL record: %w", err)
	}
	canonical, err := canonicaljson.Marshal(value)
	if err != nil {
		return fmt.Errorf("canonicalize JSONL record: %w", err)
	}
	if string(canonical) != string(bytesTrimSpace(data)) {
		return fmt.Errorf("JSONL record is not canonical")
	}
	return nil
}
