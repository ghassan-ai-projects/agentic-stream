package eventlog

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
)

// RequireSchemaValidation makes ingress validate every envelope against the
// durable event_schemas registry before it can enter event_log.
func (l *EventLog) RequireSchemaValidation() *EventLog {
	l.requireSchemas = true
	return l
}

// ValidateEnvelope checks an envelope against the durable registered schema.
func (l *EventLog) ValidateEnvelope(ctx context.Context, env contractsv1.Envelope) error {
	if !l.requireSchemas {
		return nil
	}
	return validateEnvelopeAgainstSchema(ctx, l.db, env)
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func validateEnvelopeAgainstSchema(ctx context.Context, queryer queryRower, env contractsv1.Envelope) error {
	schemaJSON, err := loadEventSchemaJSON(ctx, queryer, env.Type, env.SchemaVersion)
	if err != nil {
		return err
	}
	schema, err := domain.DecodeEventSchema(schemaJSON)
	if err != nil {
		return err
	}
	return schema.CheckPayload(env.Data)
}

func loadEventSchemaJSON(ctx context.Context, queryer queryRower, eventType, schemaVersion string) ([]byte, error) {
	var schemaJSON []byte
	if err := queryer.QueryRowContext(ctx, "SELECT schema_json FROM event_schemas WHERE event_type = ? AND schema_version = ? AND status = 'active'", eventType, schemaVersion).Scan(&schemaJSON); err != nil {
		return nil, fmt.Errorf("event schema %s/%s is not registered: %w", eventType, schemaVersion, err)
	}
	return schemaJSON, nil
}
