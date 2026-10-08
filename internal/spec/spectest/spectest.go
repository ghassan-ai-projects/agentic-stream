// Package spectest is test support for the spec module: it registers a
// built-in event schema directly, which production does when a spec is
// deployed. Import it from tests only.
package spectest

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/store"
)

// EventSchemaJSON returns the structural schema of a built-in definition.
func EventSchemaJSON(definition domain.EventSchema) ([]byte, error) {
	schema, err := domain.EventSchemaJSON(definition)
	if err != nil {
		return nil, fmt.Errorf("render event schema: %w", err)
	}
	return schema, nil
}

// RegisterEventSchema stores one event schema version in the caller's
// transaction; re-registering identical bytes is allowed.
func RegisterEventSchema(ctx context.Context, tx *sql.Tx, definition domain.EventSchema, schemaJSON []byte, now string) error {
	if err := store.RegisterEventSchema(ctx, tx, definition, schemaJSON, now); err != nil {
		return fmt.Errorf("register event schema: %w", err)
	}
	return nil
}
