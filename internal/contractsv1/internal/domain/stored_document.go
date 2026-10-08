package domain

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

type DigestDomain = canonicaljson.Domain

var (
	ErrDocumentJSON   = errors.New("document is not unambiguous JSON")
	ErrDocumentSchema = errors.New("document violates its schema")
	ErrDocumentDigest = errors.New("document does not match its digest")
)

func DecodeDocumentJSON(raw []byte) (map[string]any, error) {
	canonical, err := canonicaljson.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDocumentJSON, err)
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil || document == nil {
		return nil, fmt.Errorf("%w: not a JSON object", ErrDocumentJSON)
	}
	return document, nil
}

func DecodeDocument(raw []byte, schema SchemaName) (map[string]any, error) {
	document, err := DecodeDocumentJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", schema, err)
	}
	if err := Validate(schema, document); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDocumentSchema, err)
	}
	return document, nil
}

func VerifyDocumentDigest(digestDomain DigestDomain, document map[string]any, sum []byte) bool {
	if !canonicaljson.HasSumLength(sum) {
		return false
	}
	if digestDomain == canonicaljson.DomainIntent {
		return VerifyIntentDigest(document) && canonicaljson.VerifySum(digestDomain, withoutIntentDigest(document), sum)
	}
	return canonicaljson.VerifySum(digestDomain, document, sum)
}

func VerifyStoredDocument(schema SchemaName, digestDomain DigestDomain, raw, sum []byte) (map[string]any, error) {
	document, err := DecodeDocument(raw, schema)
	if err != nil {
		return nil, err
	}
	if !VerifyDocumentDigest(digestDomain, document, sum) {
		return nil, fmt.Errorf("%w: %s", ErrDocumentDigest, schema)
	}
	return document, nil
}
