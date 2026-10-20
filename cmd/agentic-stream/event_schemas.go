package main

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const eventSchemasFlagUsage = "directory of event-schema JSON files added to the built-in event schemas"

func eventSchemaCompileOptions(dir string) ([]spec.CompileOption, error) {
	if dir == "" {
		return nil, nil
	}
	schemas, err := spec.LoadEventSchemas(dir)
	if err != nil {
		return nil, fmt.Errorf("--event-schemas: %w", err)
	}
	return []spec.CompileOption{spec.WithEventSchemas(schemas)}, nil
}
