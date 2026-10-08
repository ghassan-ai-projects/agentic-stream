// Package spectest is test support for the spec module: it registers a
// built-in event schema directly, which production does when a spec is
// deployed. Import it from tests only.
package spectest

import (
	"context"
	"database/sql"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/store"
)

// EventSchemaJSON returns the structural schema of a built-in definition.
func EventSchemaJSON(definition domain.EventSchema) ([]byte, error) {
	return domain.EventSchemaJSON(definition) //nolint:wrapcheck // The domain names the failed rule.
}

// RegisterEventSchema stores one event schema version in the caller's
// transaction; re-registering identical bytes is allowed.
func RegisterEventSchema(ctx context.Context, tx *sql.Tx, definition domain.EventSchema, schemaJSON []byte, now string) error {
	return store.RegisterEventSchema(ctx, tx, definition, schemaJSON, now) //nolint:wrapcheck // The store names the failed step.
}
