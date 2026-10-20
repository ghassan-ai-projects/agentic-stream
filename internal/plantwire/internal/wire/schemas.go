package wire

import (
	"embed"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

const (
	schemaDir      = "plantwire-contract/schemas"
	schemaSuffix   = ".schema.json"
	schemaIDPrefix = "urn:plant-wire:v1:"
	kindFileSchema = "KIND"
	boardSchema    = "BOARD"
	maxQuoted      = 64
)

var ErrUnknownType = errors.New("plantwire: unknown record type")

//go:embed plantwire-contract/schemas/*.schema.json
var schemaFiles embed.FS

var compiledSchemas = sync.OnceValues(compileSchemas)

func compileSchemas() (map[string]*jsonschema.Schema, error) {
	entries, err := schemaFiles.ReadDir(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("plantwire: read the embedded schemas: %w", err)
	}
	compiled := make(map[string]*jsonschema.Schema, len(entries))
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), schemaSuffix)
		if compiled[name], err = compileSchema(name, entry.Name()); err != nil {
			return nil, err
		}
	}
	return compiled, nil
}

func compileSchema(name, file string) (*jsonschema.Schema, error) {
	raw, err := schemaFiles.ReadFile(schemaDir + "/" + file)
	if err != nil {
		return nil, fmt.Errorf("plantwire: read the embedded schema %s: %w", name, err)
	}
	schema, err := canonicaljson.CompileSchemaJSON(schemaIDPrefix+name, raw)
	if err != nil {
		return nil, fmt.Errorf("plantwire: compile the embedded schema %s: %w", name, err)
	}
	return schema, nil
}

func recordSchema(recordType string) (*jsonschema.Schema, error) {
	if recordType == kindFileSchema || recordType == boardSchema {
		return nil, fmt.Errorf("%w: %q names a file, not a record", ErrUnknownType, recordType)
	}
	return schemaOf(recordType)
}

func schemaOf(name string) (*jsonschema.Schema, error) {
	compiled, err := compiledSchemas()
	if err != nil {
		return nil, err
	}
	schema, known := compiled[name]
	if !known {
		return nil, fmt.Errorf("%w: %q", ErrUnknownType, clip(name))
	}
	return schema, nil
}

func clip(text string) string {
	if len(text) <= maxQuoted {
		return text
	}
	return text[:maxQuoted] + "..."
}
