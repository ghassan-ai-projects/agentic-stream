package store_test

import (
	"bytes"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSchemaRegistrationIsImmutableAndTransactionScoped(t *testing.T) {
	ctx := t.Context()
	db := storagetest.OpenTemp(t)

	definition, ok := domain.LookupEventSchema("bay.air_temp.observed/1.0")
	if !ok {
		t.Fatal("builtin schema missing")
	}
	raw, err := domain.EventSchemaJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("abort caller transaction")
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := store.RegisterEventSchema(ctx, tx, definition, raw, "first"); err != nil {
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
			return store.RegisterEventSchema(ctx, tx, definition, raw, timestamp)
		}); err != nil {
			t.Fatal(err)
		}
	}
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return store.RegisterEventSchema(ctx, tx, definition, append(bytes.Clone(raw), ' '), "conflict")
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
	definition := domain.EventSchema{Ref: "test/1", EventType: "test", SchemaVersion: "1"}
	if err := store.RegisterEventSchema(t.Context(), nil, definition, nil, "now"); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty bytes = %v", err)
	}
	if err := store.RegisterEventSchema(t.Context(), nil, definition, []byte("invalid"), "now"); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("invalid JSON = %v", err)
	}
}
