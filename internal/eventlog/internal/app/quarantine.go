package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
)

func (s *Service) Quarantine(ctx context.Context, tenantID string, env map[string]any, reason string, now time.Time) error {
	if err := domain.ValidQuarantine(tenantID, reason, now); err != nil {
		return err
	}
	payload, err := domain.NewQuarantinePayload(env)
	if err != nil {
		return err
	}
	conflict, err := s.quarantineInUnit(ctx, payload, tenantID, reason, now)
	if err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("event id %s has conflicting quarantined payload", payload.EventID)
	}
	return nil
}

func (s *Service) quarantineInUnit(ctx context.Context, payload domain.QuarantinePayload, tenantID, reason string, now time.Time) (bool, error) {
	conflict := false
	if err := s.store.Unit(ctx, func(u *store.Unit) error {
		var err error
		conflict, err = s.persistQuarantine(ctx, u, payload, tenantID, reason, now)
		return err
	}); err != nil {
		return false, fmt.Errorf("quarantine event transaction: %w", err)
	}
	return conflict, nil
}

func (s *Service) persistQuarantine(ctx context.Context, u *store.Unit, payload domain.QuarantinePayload, tenantID, reason string, now time.Time) (bool, error) {
	existing, err := u.QuarantineDigest(ctx, tenantID, payload.EventID)
	if err != nil {
		return false, fmt.Errorf("read quarantine of %s: %w", payload.EventID, err)
	}
	if existing != nil && payload.ConflictingPayload(existing) {
		return true, rejectConflict(ctx, u, tenantID, payload.EventID, now)
	}
	updated, err := u.UpsertQuarantine(ctx, payload, tenantID, reason, now)
	if err != nil {
		return false, fmt.Errorf("quarantine %s: %w", payload.EventID, err)
	}
	if updated == 0 {
		return true, rejectConflict(ctx, u, tenantID, payload.EventID, now)
	}
	return false, s.recordOverflow(ctx, u, payload, tenantID, now)
}

func rejectConflict(ctx context.Context, u *store.Unit, tenantID, eventID string, now time.Time) error {
	if err := u.RejectQuarantineConflict(ctx, tenantID, eventID, now); err != nil {
		return fmt.Errorf("reject conflicting quarantine of %s: %w", eventID, err)
	}
	return nil
}

func (s *Service) recordOverflow(ctx context.Context, u *store.Unit, payload domain.QuarantinePayload, tenantID string, now time.Time) error {
	status, err := u.QuarantineStatus(ctx, tenantID, payload.EventID)
	if err != nil {
		return fmt.Errorf("read quarantine status of %s: %w", payload.EventID, err)
	}
	if status != "rejected" {
		return nil
	}
	if err := u.InsertOverflowGap(ctx, payload.OverflowGapID(), tenantID, now); err != nil {
		return fmt.Errorf("record overflow gap of %s: %w", payload.EventID, err)
	}
	return nil
}

func (s *Service) QuarantineEnvelope(ctx context.Context, tenantID string, env contractsv1.Envelope, reason string, now time.Time) error {
	document, err := envelopeDocument(env)
	if err != nil {
		return err
	}
	return s.Quarantine(ctx, tenantID, document, reason, now)
}

func quarantinePayloadOf(env contractsv1.Envelope) (domain.QuarantinePayload, error) {
	document, err := envelopeDocument(env)
	if err != nil {
		return domain.QuarantinePayload{}, err
	}
	payload, err := domain.NewQuarantinePayload(document)
	if err != nil {
		return domain.QuarantinePayload{}, fmt.Errorf("quarantine payload of %s: %w", env.ID, err)
	}
	return payload, nil
}

func envelopeDocument(env contractsv1.Envelope) (map[string]any, error) {
	encoded, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal quarantined envelope: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return nil, fmt.Errorf("decode quarantined envelope: %w", err)
	}
	return document, nil
}

func (s *Service) QuarantineRaw(ctx context.Context, tenantID, eventID string, raw []byte, reason string, now time.Time) error {
	return s.Quarantine(ctx, tenantID, map[string]any{
		"id": eventID, "type": "", "schema_version": "", "source": "",
		"data": map[string]any{"raw": string(raw)},
	}, reason, now)
}

func (s *Service) ReleaseQuarantine(ctx context.Context, tenantID, eventID string, now time.Time) error {
	if err := domain.ValidRelease(tenantID, eventID, now); err != nil {
		return err
	}
	if err := s.store.Unit(ctx, func(u *store.Unit) error {
		return u.ReleaseQuarantined(ctx, tenantID, eventID, now)
	}); err != nil {
		return fmt.Errorf("release quarantine transaction: %w", err)
	}
	return nil
}

func (s *Service) RedriveQuarantine(ctx context.Context, tenantID, eventID string, now time.Time) (domain.LogPosition, error) {
	if err := domain.ValidRelease(tenantID, eventID, now); err != nil {
		return -1, err
	}
	position := domain.LogPosition(-1)
	if err := s.store.Unit(ctx, func(u *store.Unit) error {
		var err error
		position, err = s.redrive(ctx, u, tenantID, eventID, now)
		return err
	}); err != nil {
		return -1, fmt.Errorf("redrive quarantine transaction: %w", err)
	}
	return position, nil
}

func (s *Service) redrive(ctx context.Context, u *store.Unit, tenantID, eventID string, now time.Time) (domain.LogPosition, error) {
	env, err := u.ReleasedEnvelope(ctx, tenantID, eventID)
	if err != nil {
		return -1, fmt.Errorf("read released envelope %s: %w", eventID, err)
	}
	if err := s.admit(ctx, u, tenantID, env); err != nil {
		return -1, fmt.Errorf("validate released envelope: %w", err)
	}
	position, err := s.appendOne(ctx, u, tenantID, env)
	if err != nil {
		return -1, fmt.Errorf("append released event: %w", err)
	}
	if err := u.MarkRedriven(ctx, tenantID, eventID, now); err != nil {
		return -1, fmt.Errorf("mark %s redriven: %w", eventID, err)
	}
	return position, nil
}

func (s *Service) Quarantined(ctx context.Context, tenantID string) ([]domain.QuarantineRecord, error) {
	records, err := s.store.Quarantined(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("quarantined records of tenant %s: %w", tenantID, err)
	}
	return records, nil
}
