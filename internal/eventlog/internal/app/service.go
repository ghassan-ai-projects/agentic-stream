package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

type Service struct {
	store          store.Store
	clk            sources.Clock
	requireSchemas bool
	schemas        sync.Map
}

type schemaRef struct{ eventType, version string }

func New(clk sources.Clock, st store.Store) *Service {
	clk = sources.OrPhysical(clk)
	return &Service{store: st, clk: clk}
}

func (s *Service) RequireSchemaValidation() *Service {
	s.requireSchemas = true
	return s
}

func (s *Service) CurrentPosition(ctx context.Context, tenantID string) (domain.LogPosition, error) {
	return s.store.CurrentPosition(ctx, tenantID) //nolint:wrapcheck // Store owns the query error context.
}

func (s *Service) ValidateEnvelope(ctx context.Context, env contractsv1.Envelope) error {
	if !s.requireSchemas {
		return nil
	}
	if schema, cached := s.cachedSchema(env); cached {
		return checkPayload(schema, env)
	}
	return s.store.Unit(ctx, func(u *store.Unit) error {
		return s.validateSchema(ctx, u, env)
	})
}

func (s *Service) validateSchema(ctx context.Context, u *store.Unit, env contractsv1.Envelope) error {
	schema, err := s.registeredSchema(ctx, u, env)
	if err != nil {
		return err
	}
	return checkPayload(schema, env)
}

func (s *Service) cachedSchema(env contractsv1.Envelope) (domain.EventSchema, bool) {
	cached, ok := s.schemas.Load(schemaRef{env.Type, env.SchemaVersion})
	if !ok {
		return domain.EventSchema{}, false
	}
	return cached.(domain.EventSchema), true
}

func (s *Service) registeredSchema(ctx context.Context, u *store.Unit, env contractsv1.Envelope) (domain.EventSchema, error) {
	if schema, cached := s.cachedSchema(env); cached {
		return schema, nil
	}
	schemaJSON, err := u.LoadEventSchemaJSON(ctx, env.Type, env.SchemaVersion)
	if err != nil {
		return domain.EventSchema{}, fmt.Errorf("load schema of %s: %w", env.ID, err)
	}
	schema, err := domain.DecodeEventSchema(schemaJSON)
	if err != nil {
		return domain.EventSchema{}, fmt.Errorf("decode schema %s/%s: %w", env.Type, env.SchemaVersion, err)
	}
	s.schemas.Store(schemaRef{env.Type, env.SchemaVersion}, schema)
	return schema, nil
}

func checkPayload(schema domain.EventSchema, env contractsv1.Envelope) error {
	if err := schema.CheckPayload(env.Data); err != nil {
		return fmt.Errorf("check payload of %s: %w", env.ID, err)
	}
	return nil
}

func (s *Service) ReadEntityEvents(ctx context.Context, window domain.EntityWindow, visit func(domain.EntityEvent) (bool, error)) error {
	return s.store.ReadEntityEvents(ctx, window, visit) //nolint:wrapcheck // Store owns the query error context.
}

func (s *Service) admit(ctx context.Context, u *store.Unit, tenantID string, env contractsv1.Envelope) error {
	if err := contractsv1.ValidateEnvelope(env, tenantID); err != nil {
		return fmt.Errorf("validate envelope: %w", err)
	}
	if !s.requireSchemas {
		return nil
	}
	return s.validateSchema(ctx, u, env)
}

func (s *Service) appendOne(ctx context.Context, u *store.Unit, tenantID string, env contractsv1.Envelope) (domain.LogPosition, error) {
	if _, err := contractsv1.ParseTraceContext(env.Traceparent, env.Tracestate); err != nil {
		return -1, fmt.Errorf("validate trace context: %w", err)
	}
	body, err := domain.EncodeEventBody(env.Data, env.Quality)
	if err != nil {
		return -1, fmt.Errorf("encode event %s: %w", env.ID, err)
	}
	position, err := u.InsertEvent(ctx, tenantID, env, body, s.clk.Now())
	if err != nil || position >= 0 {
		return position, err
	}
	return -1, s.quarantineConflictingDuplicate(ctx, u, tenantID, env, body)
}

func (s *Service) quarantineConflictingDuplicate(ctx context.Context, u *store.Unit, tenantID string, env contractsv1.Envelope, body domain.EncodedEvent) error {
	logged, err := u.LoggedEvent(ctx, tenantID, env.ID)
	if err != nil {
		return fmt.Errorf("compare duplicate event %s: %w", env.ID, err)
	}
	if logged.SameEvent(loggedForm(env, body)) {
		return nil
	}
	payload, err := quarantinePayloadOf(env)
	if err != nil {
		return err
	}
	if _, err := s.persistQuarantine(ctx, u, payload, tenantID, domain.ReasonEventIDConflict, s.clk.Now()); err != nil {
		return fmt.Errorf("quarantine conflicting event %s: %w", env.ID, err)
	}
	return nil
}

func loggedForm(env contractsv1.Envelope, body domain.EncodedEvent) domain.LoggedEvent {
	return domain.LoggedEvent{
		EventType: env.Type, EntityType: env.Entity.Type, EntityID: env.Entity.ID,
		EventTime: kernel.FormatTime(env.EventTime), PayloadSHA256: body.PayloadSHA256,
	}
}
