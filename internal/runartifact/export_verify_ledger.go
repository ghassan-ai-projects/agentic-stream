package runartifact

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func verifyLedgerFiles(dir string) error {
	for _, name := range exportedJSONL {
		data, err := readArtifactFile(dir, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := verifyJSONL(data); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
		if err := verifyLedgerRows(name, data); err != nil {
			return fmt.Errorf("verify %s ledger bindings: %w", name, err)
		}
	}
	return nil
}

func verifyLedgerRows(name string, data []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return fmt.Errorf("decode ledger row: %w", err)
		}
		if err := verifyLedgerRow(name, row); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan ledger rows: %w", err)
	}
	return nil
}

func verifyLedgerRow(name string, row map[string]any) error {
	switch name {
	case "commands.jsonl":
		return verifyRowDigest(row, "command_json", "command_sha256", canonicaljson.DomainCommand)
	case "decisions.jsonl":
		return verifyRowDigest(row, "raw_json", "decision_sha256", canonicaljson.DomainDecision)
	case "situations.jsonl":
		return verifyRowDigest(row, "snapshot_json", "snapshot_sha256", canonicaljson.DomainSnapshot)
	case "authority-events.jsonl", "safety-events.jsonl":
		return verifyRawJSONRowDigest(row, "details_json", "details_sha256")
	default:
		return nil
	}
}

func verifyRawJSONRowDigest(row map[string]any, documentField, digestField string) error {
	document, ok := row[documentField].(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not a JSON object", documentField)
	}
	storedDigest, err := decodeRowDigest(row, digestField)
	if err != nil {
		return err
	}
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		return fmt.Errorf("canonicalize %s: %w", documentField, err)
	}
	expected := sha256.Sum256(canonical)
	if string(storedDigest) != string(expected[:]) {
		return fmt.Errorf("%s does not match %s", digestField, documentField)
	}
	return nil
}

func verifyRowDigest(row map[string]any, documentField, digestField string, domain canonicaljson.Domain) error {
	document, ok := row[documentField].(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not a JSON object", documentField)
	}
	storedDigest, err := decodeRowDigest(row, digestField)
	if err != nil {
		return err
	}
	expected, err := canonicaljson.Digest(domain, document)
	if err != nil {
		return fmt.Errorf("digest %s: %w", documentField, err)
	}
	expectedDigest, err := canonicaljson.DecodeDigest(expected)
	if err != nil {
		return fmt.Errorf("decode expected %s: %w", digestField, err)
	}
	if string(storedDigest) != string(expectedDigest) {
		return fmt.Errorf("%s does not match %s", digestField, documentField)
	}
	return nil
}

func decodeRowDigest(row map[string]any, field string) ([]byte, error) {
	encoded, ok := row[field].(string)
	if !ok {
		return nil, fmt.Errorf("%s is not a base64 BLOB", field)
	}
	digest, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(digest) != sha256.Size {
		return nil, fmt.Errorf("%s is not a 32-byte base64 digest", field)
	}
	return digest, nil
}
