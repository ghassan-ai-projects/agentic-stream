package eventschema

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Register stores one immutable event schema version. Re-registering the same
// event type and version is allowed only when the bytes are identical.
func Register(ctx context.Context, tx *sql.Tx, definition Definition, schemaJSON []byte, now string) error {
	if definition.Ref == "" || definition.EventType == "" || definition.SchemaVersion == "" || len(schemaJSON) == 0 || now == "" {
		return fmt.Errorf("event schema identity, bytes, and time are required")
	}
	if !json.Valid(schemaJSON) {
		return fmt.Errorf("event schema %s is not valid JSON", definition.Ref)
	}
	digest := sha256.Sum256(schemaJSON)
	var existing []byte
	err := tx.QueryRowContext(ctx, "SELECT schema_sha256 FROM event_schemas WHERE event_type = ? AND schema_version = ?", definition.EventType, definition.SchemaVersion).Scan(&existing)
	if err == nil {
		if string(existing) != string(digest[:]) {
			return fmt.Errorf("event schema %s has conflicting bytes", definition.Ref)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check event schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_schemas (schema_id, event_type, schema_version, schema_json, schema_sha256, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'active', ?)`, definition.Ref, definition.EventType, definition.SchemaVersion, schemaJSON, digest[:], now); err != nil {
		return fmt.Errorf("register event schema: %w", err)
	}
	return nil
}
