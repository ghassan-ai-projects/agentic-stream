package spectest_test

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/spectest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func zoneTempSchema(t *testing.T) (spec.EventSchema, []byte) {
	t.Helper()
	definition, ok := spec.LookupEventSchema("zone.temp.observed/1.0")
	if !ok {
		t.Fatal("built-in schema missing")
	}
	schemaJSON, err := spectest.EventSchemaJSON(definition)
	if err != nil {
		t.Fatal(err)
	}
	return definition, schemaJSON
}

func TestEventSchemaJSONRendersTheStructuralSchema(t *testing.T) {
	t.Parallel()
	definition, schemaJSON := zoneTempSchema(t)

	var document struct {
		Type                 string                    `json:"type"`
		AdditionalProperties bool                      `json:"additionalProperties"`
		Properties           map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schemaJSON, &document); err != nil {
		t.Fatal(err)
	}
	if document.Type != "object" || document.AdditionalProperties || len(document.Properties) != len(definition.Fields) {
		t.Fatalf("schema = %s, want a closed object with the %d declared fields", schemaJSON, len(definition.Fields))
	}
}

func TestRegisterEventSchemaIsIdempotent(t *testing.T) {
	t.Parallel()
	definition, schemaJSON := zoneTempSchema(t)
	db := storagetest.OpenTemp(t)

	for range 2 {
		if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return spectest.RegisterEventSchema(t.Context(), tx, definition, schemaJSON, "2026-10-08T00:00:00Z")
		}); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_schemas WHERE schema_id = ?", definition.Ref).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("registered rows = %d, want 1", count)
	}
}

func TestRegisterEventSchemaRefusesDifferentBytesForARegisteredVersion(t *testing.T) {
	t.Parallel()
	definition, schemaJSON := zoneTempSchema(t)
	db := storagetest.OpenTemp(t)
	register := func(payload []byte) error {
		return db.WithTx(t.Context(), func(tx *sql.Tx) error {
			return spectest.RegisterEventSchema(t.Context(), tx, definition, payload, "2026-10-08T00:00:00Z")
		})
	}
	if err := register(schemaJSON); err != nil {
		t.Fatal(err)
	}

	err := register([]byte(`{"type":"object"}`))

	if err == nil || !strings.Contains(err.Error(), "register event schema: ") || !strings.Contains(err.Error(), "conflicting bytes") {
		t.Fatalf("err = %v, want the conflicting bytes refusal wrapped by register event schema", err)
	}
}
