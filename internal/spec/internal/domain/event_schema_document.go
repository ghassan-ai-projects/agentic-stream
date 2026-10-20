package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const maxEventSchemaDocumentDepth = 8

func decodeEventSchemaDocument(source string, document []byte) (map[string]registryEntry, error) {
	if err := requireUniqueKeys(document); err != nil {
		return nil, fmt.Errorf("decode event schemas %s: %w", source, err)
	}
	entries, err := decodeEventSchemaEntries(document)
	if err != nil {
		return nil, fmt.Errorf("decode event schemas %s: %w", source, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("decode event schemas %s: the document declares no event schema", source)
	}
	return entries, nil
}

func decodeEventSchemaEntries(document []byte) (map[string]registryEntry, error) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	var entries map[string]registryEntry
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode schema document: %w", err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("trailing data after the schema document")
	}
	return entries, nil
}

func requireUniqueKeys(document []byte) error {
	return walkUniqueValue(json.NewDecoder(bytes.NewReader(document)), 0)
}

func walkUniqueValue(decoder *json.Decoder, depth int) error {
	if depth > maxEventSchemaDocumentDepth {
		return fmt.Errorf("the document nests deeper than %d levels", maxEventSchemaDocumentDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read schema document: %w", err)
	}
	switch token {
	case json.Delim('{'):
		return walkUniqueObject(decoder, depth)
	case json.Delim('['):
		return walkArray(decoder, depth)
	}
	return nil
}

func walkUniqueObject(decoder *json.Decoder, depth int) error {
	seen := make(map[string]bool)
	for decoder.More() {
		if err := readUniqueKey(decoder, seen); err != nil {
			return err
		}
		if err := walkUniqueValue(decoder, depth+1); err != nil {
			return err
		}
	}
	return closeDelimiter(decoder)
}

func readUniqueKey(decoder *json.Decoder, seen map[string]bool) error {
	key, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("read schema document key: %w", err)
	}
	name, _ := key.(string)
	if seen[name] {
		return fmt.Errorf("duplicate key %q", name)
	}
	seen[name] = true
	return nil
}

func walkArray(decoder *json.Decoder, depth int) error {
	for decoder.More() {
		if err := walkUniqueValue(decoder, depth+1); err != nil {
			return err
		}
	}
	return closeDelimiter(decoder)
}

func closeDelimiter(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("read schema document: %w", err)
	}
	return nil
}
