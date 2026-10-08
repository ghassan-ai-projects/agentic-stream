package spectest_test

import (
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/spectest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRegisterEventSchemaIsIdempotent(t *testing.T) {
	t.Parallel()
	definition, ok := spec.LookupEventSchema("zone.temp.observed/1.0")
	if !ok {
		t.Fatal("built-in schema missing")
	}
	schemaJSON, err := spectest.EventSchemaJSON(definition)
	if err != nil || len(schemaJSON) == 0 {
		t.Fatalf("schema json: %v", err)
	}
	db := storagetest.OpenTemp(t)

	for range 2 {
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return spectest.RegisterEventSchema(t.Context(), tx, definition, schemaJSON, "2026-10-08T00:00:00Z")
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
}
