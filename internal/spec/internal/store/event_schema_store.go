package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
)

// Register stores one immutable event schema version. Re-registering the same
// event type and version is allowed only when the bytes are identical.
func RegisterEventSchema(ctx context.Context, tx *sql.Tx, definition domain.EventSchema, schemaJSON []byte, now string) error {
	if definition.Ref == "" || definition.EventType == "" || definition.SchemaVersion == "" || len(schemaJSON) == 0 || now == "" {
		return fmt.Errorf("event schema identity, bytes, and time are required")
	}
	if !json.Valid(schemaJSON) {
		return fmt.Errorf("event schema %s is not valid JSON", definition.Ref)
	}
	digest := canonicaljson.Sum(schemaJSON)
	registered, err := checkRegisteredDigest(ctx, tx, definition, digest)
	if err != nil || registered {
		return err
	}
	return insertSchema(ctx, tx, definition, schemaJSON, digest, now)
}

func insertSchema(ctx context.Context, tx *sql.Tx, definition domain.EventSchema, schemaJSON, digest []byte, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_schemas (schema_id, event_type, schema_version, schema_json, schema_sha256, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'active', ?)`, definition.Ref, definition.EventType, definition.SchemaVersion, schemaJSON, digest, now); err != nil {
		return fmt.Errorf("register event schema: %w", err)
	}
	return nil
}

// checkRegisteredDigest reports whether the schema version is already
// registered, failing when its stored bytes differ from digest.
func checkRegisteredDigest(ctx context.Context, tx *sql.Tx, definition domain.EventSchema, digest []byte) (bool, error) {
	var existing []byte
	err := tx.QueryRowContext(ctx, "SELECT schema_sha256 FROM event_schemas WHERE event_type = ? AND schema_version = ?", definition.EventType, definition.SchemaVersion).Scan(&existing)
	if err == sql.ErrNoRows { //nolint:errorlint // Scan returns sql.ErrNoRows unwrapped.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check event schema: %w", err)
	}
	if string(existing) != string(digest) {
		return false, fmt.Errorf("event schema %s has conflicting bytes", definition.Ref)
	}
	return true, nil
}
