package store

import (
	"context"
	"strings"
	"testing"
)

func registerSchema(t *testing.T, st Store, status string) {
	t.Helper()
	_, err := st.DB.ExecContext(t.Context(), `INSERT INTO event_schemas (schema_id, event_type, schema_version, schema_json, schema_sha256, status, created_at)
		VALUES ('schema-1', 'sensor.temperature', '1.0', X'7B7D', zeroblob(32), ?, '2026-08-12T12:00:00Z')`, status)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadEventSchemaReturnsOnlyAnActiveRegisteredSchema(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, eventType, version, status string
		want                             string
	}{
		{"active schema", "sensor.temperature", "1.0", "active", ""},
		{"retired schema", "sensor.temperature", "1.0", "retired", "event schema sensor.temperature/1.0 is not registered"},
		{"unknown type", "unknown.type", "1.0", "active", "event schema unknown.type/1.0 is not registered"},
		{"unknown version", "sensor.temperature", "9.9", "active", "event schema sensor.temperature/9.9 is not registered"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := newStore(t)
			registerSchema(t, st, tc.status)
			inUnit(t, st, func(ctx context.Context, u *Unit) error {
				schema, err := u.LoadEventSchemaJSON(ctx, tc.eventType, tc.version)
				if tc.want == "" && (err != nil || string(schema) != "{}") {
					t.Fatalf("schema = %s err=%v, want {}", schema, err)
				}
				if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
					t.Fatalf("err = %v, want %q", err, tc.want)
				}
				return nil
			})
		})
	}
}
