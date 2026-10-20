package main

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const eventSchemasFlag = "event-schemas"

func registerEventSchemasFlag(root *cobra.Command) {
	root.PersistentFlags().String(eventSchemasFlag, "", "directory of event-schema JSON files added to the built-in event schemas")
}

func loadEventSchemas(cmd *cobra.Command) (*spec.EventSchemas, error) {
	dir := eventSchemaDirectory(cmd)
	if dir == "" {
		return spec.BuiltinEventSchemas(), nil
	}
	schemas, err := spec.LoadEventSchemas(dir)
	if err != nil {
		return nil, fmt.Errorf("--%s: %w", eventSchemasFlag, err)
	}
	return schemas, nil
}

func loadLoggedEventSchemas(cmd *cobra.Command) (*spec.EventSchemas, error) {
	schemas, err := loadEventSchemas(cmd)
	if err != nil {
		return nil, err
	}
	digest, err := schemas.Digest()
	if err != nil {
		return nil, fmt.Errorf("digest event schemas: %w", err)
	}
	slog.Info("event schemas loaded", "event_schemas_digest", digest, "event_schemas_dir", eventSchemaDirectory(cmd))
	return schemas, nil
}

func eventSchemaDirectory(cmd *cobra.Command) string {
	flag := cmd.Flags().Lookup(eventSchemasFlag)
	if flag == nil {
		return ""
	}
	return flag.Value.String()
}
