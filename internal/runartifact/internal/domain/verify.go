package domain

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const maxJSONLLineBytes = 8 * 1024 * 1024

// VerifyDocument requires one canonical JSON value.
func VerifyDocument(data []byte) error {
	return requireCanonical(trimmed(data), "stored JSON", "JSON")
}

// VerifyLedger requires canonical JSON Lines with no blank record, then checks
// the ledger's own digest bindings.
func VerifyLedger(name string, data []byte) error {
	if err := eachLine(data, verifyLine); err != nil {
		return fmt.Errorf("verify %s: %w", name, err)
	}
	if err := eachLine(data, func(line []byte) error { return verifyRow(name, line) }); err != nil {
		return fmt.Errorf("verify %s ledger bindings: %w", name, err)
	}
	return nil
}

func eachLine(data []byte, check func([]byte) error) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		if err := check(scanner.Bytes()); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan JSONL: %w", err)
	}
	return nil
}

func verifyLine(line []byte) error {
	if len(trimmed(line)) == 0 {
		return fmt.Errorf("blank JSONL record")
	}
	return requireCanonical(line, "JSONL record", "JSONL record")
}

// requireCanonical decodes data and requires it to equal its canonical form;
// what names the value in decode errors, subject in the canonical-form error.
func requireCanonical(data []byte, what, subject string) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode %s: %w", what, err)
	}
	canonical, err := canonicaljson.Marshal(value)
	if err != nil {
		return fmt.Errorf("canonicalize %s: %w", what, err)
	}
	if string(canonical) != string(trimmed(data)) {
		return fmt.Errorf("%s is not canonical", subject)
	}
	return nil
}

func verifyRow(name string, line []byte) error {
	var row map[string]any
	if err := json.Unmarshal(line, &row); err != nil {
		return fmt.Errorf("decode ledger row: %w", err)
	}
	return VerifyLedgerRow(name, row)
}

// VerifyLedgerRow checks the digest a ledger row stores over its own document.
func VerifyLedgerRow(name string, row map[string]any) error {
	switch name {
	case FileCommands:
		return verifyRowDigest(row, "command_json", "command_sha256", canonicaljson.DomainCommand)
	case FileDecisions:
		return verifyRowDigest(row, "raw_json", "decision_sha256", canonicaljson.DomainDecision)
	case FileSituations:
		return verifyRowDigest(row, "snapshot_json", "snapshot_sha256", canonicaljson.DomainSnapshot)
	case FileAuthority, FileSafety:
		return verifyRowDigest(row, "details_json", "details_sha256", "")
	default:
		return nil
	}
}

// verifyRowDigest recomputes the digest of a row's JSON document. An empty
// domain means a plain SHA-256 over the canonical bytes.
func verifyRowDigest(row map[string]any, documentField, digestField string, domain canonicaljson.Domain) error {
	document, ok := row[documentField].(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not a JSON object", documentField)
	}
	stored, err := decodeRowDigest(row, digestField)
	if err != nil {
		return err
	}
	expected, err := expectedDigest(document, documentField, domain)
	if err != nil || string(stored) == string(expected) {
		return err
	}
	return fmt.Errorf("%s does not match %s", digestField, documentField)
}

func expectedDigest(document map[string]any, documentField string, domain canonicaljson.Domain) ([]byte, error) {
	if domain == "" {
		return plainDigest(document, documentField)
	}
	sum, err := canonicaljson.DigestSum(domain, document)
	if err != nil {
		return nil, fmt.Errorf("digest %s: %w", documentField, err)
	}
	return sum, nil
}

func plainDigest(document map[string]any, documentField string) ([]byte, error) {
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize %s: %w", documentField, err)
	}
	return canonicaljson.Sum(canonical), nil
}

func decodeRowDigest(row map[string]any, field string) ([]byte, error) {
	encoded, ok := row[field].(string)
	if !ok {
		return nil, fmt.Errorf("%s is not a base64 BLOB", field)
	}
	digest, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !canonicaljson.HasSumLength(digest) {
		return nil, fmt.Errorf("%s is not a 32-byte base64 digest", field)
	}
	return digest, nil
}
