package eventschema_test

import (
	"bytes"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestSchemaRegistrationIsImmutableAndTransactionScoped(t *testing.T) {
	ctx := t.Context()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "schemas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	definition, ok := eventschema.Lookup("bay.air_temp.observed/1.0")
	if !ok {
		t.Fatal("builtin schema missing")
	}
	raw, err := eventschema.JSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("abort caller transaction")
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := eventschema.Register(ctx, tx, definition, raw, "first"); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback = %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_schemas").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back registration count = %d", count)
	}
	for _, timestamp := range []string{"first", "second"} {
		if err := db.WithTx(ctx, func(tx *sql.Tx) error {
			return eventschema.Register(ctx, tx, definition, raw, timestamp)
		}); err != nil {
			t.Fatal(err)
		}
	}
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return eventschema.Register(ctx, tx, definition, append(bytes.Clone(raw), ' '), "conflict")
	})
	if err == nil || !strings.Contains(err.Error(), "conflicting bytes") {
		t.Fatalf("byte conflict = %v", err)
	}
	var stored []byte
	var created string
	if err := db.QueryRowContext(ctx, "SELECT schema_json, created_at FROM event_schemas WHERE schema_id = ?", definition.Ref).Scan(&stored, &created); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, raw) || created != "first" {
		t.Fatalf("immutable registration changed: time=%q bytes=%q", created, stored)
	}
}

func TestSchemaRegistrationValidatesBeforeStorage(t *testing.T) {
	definition := eventschema.Definition{Ref: "test/1", EventType: "test", SchemaVersion: "1"}
	if err := eventschema.Register(t.Context(), nil, definition, nil, "now"); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty bytes = %v", err)
	}
	if err := eventschema.Register(t.Context(), nil, definition, []byte("invalid"), "now"); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("invalid JSON = %v", err)
	}
}
