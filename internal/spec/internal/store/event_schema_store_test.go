package store_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSchemaRegistrationIsImmutableAndTransactionScoped(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	complete := domain.EventSchema{Ref: "test/1", EventType: "test", SchemaVersion: "1"}
	tests := []struct {
		name       string
		definition domain.EventSchema
		schemaJSON []byte
		now        string
		wantErr    string
	}{
		{name: "no bytes", definition: complete, schemaJSON: nil, now: "now", wantErr: "required"},
		{name: "no time", definition: complete, schemaJSON: []byte("{}"), now: "", wantErr: "required"},
		{name: "no ref", definition: domain.EventSchema{EventType: "test", SchemaVersion: "1"}, schemaJSON: []byte("{}"), now: "now", wantErr: "required"},
		{name: "no event type", definition: domain.EventSchema{Ref: "test/1", SchemaVersion: "1"}, schemaJSON: []byte("{}"), now: "now", wantErr: "required"},
		{name: "no schema version", definition: domain.EventSchema{Ref: "test/1", EventType: "test"}, schemaJSON: []byte("{}"), now: "now", wantErr: "required"},
		{name: "bytes that are not JSON", definition: complete, schemaJSON: []byte("invalid"), now: "now", wantErr: "not valid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := store.RegisterEventSchema(t.Context(), nil, tt.definition, tt.schemaJSON, tt.now)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestSchemaRegistrationReportsAFailedStorageRead(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	definition, _ := domain.LookupEventSchema("bay.air_temp.observed/1.0")
	raw, err := domain.EventSchemaJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = db.WithTx(t.Context(), func(tx *sql.Tx) error {
		return store.RegisterEventSchema(ctx, tx, definition, raw, "now")
	})

	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "check event schema") {
		t.Fatalf("err = %v, want a canceled read of the registered schema", err)
	}
}
