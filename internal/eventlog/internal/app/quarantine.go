package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
)

// Quarantine records an invalid event durably without placing it in the
// executable event log. Repeated delivery increments a bounded retry count.
func (s *Service) Quarantine(ctx context.Context, tenantID string, env map[string]any, reason, now string) error {
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

// quarantineInUnit commits the quarantine delivery in one transaction; a hash
// conflict commits the rejection and is reported after commit.
func (s *Service) quarantineInUnit(ctx context.Context, payload domain.QuarantinePayload, tenantID, reason, now string) (bool, error) {
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

// persistQuarantine upserts the payload identity: a different payload under
// the same event id rejects the record; a spent retry budget records a gap.
func (s *Service) persistQuarantine(ctx context.Context, u *store.Unit, payload domain.QuarantinePayload, tenantID, reason, now string) (bool, error) {
	existing, err := u.QuarantineDigest(ctx, tenantID, payload.EventID)
	if err != nil {
		return false, err
	}
	if existing != nil && payload.ConflictingPayload(existing) {
		return true, u.RejectQuarantineConflict(ctx, tenantID, payload.EventID, now)
	}
	updated, err := u.UpsertQuarantine(ctx, payload, tenantID, reason, now)
	if err != nil {
		return false, err
	}
	if updated == 0 {
		return true, u.RejectQuarantineConflict(ctx, tenantID, payload.EventID, now)
	}
	return false, s.recordOverflow(ctx, u, payload, tenantID, now)
}

// recordOverflow records an event gap once the record's retries are spent.
func (s *Service) recordOverflow(ctx context.Context, u *store.Unit, payload domain.QuarantinePayload, tenantID, now string) error {
	status, err := u.QuarantineStatus(ctx, tenantID, payload.EventID)
	if err != nil {
		return err
	}
	if status != "rejected" {
		return nil
	}
	return u.InsertOverflowGap(ctx, payload.OverflowGapID(), tenantID, now)
}

// QuarantineEnvelope records a normalized envelope that failed validation.
func (s *Service) QuarantineEnvelope(ctx context.Context, tenantID string, env contractsv1.Envelope, reason, now string) error {
	encoded, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal quarantined envelope: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return fmt.Errorf("decode quarantined envelope: %w", err)
	}
	return s.Quarantine(ctx, tenantID, document, reason, now)
}

// QuarantineRaw preserves malformed JSON as data with a stable line identity.
func (s *Service) QuarantineRaw(ctx context.Context, tenantID, eventID string, raw []byte, reason, now string) error {
	return s.Quarantine(ctx, tenantID, map[string]any{
		"id": eventID, "type": "", "schema_version": "", "source": "",
		"data": map[string]any{"raw": string(raw)},
	}, reason, now)
}

// ReleaseQuarantine marks one record ready for an explicit re-drive.
func (s *Service) ReleaseQuarantine(ctx context.Context, tenantID, eventID, now string) error {
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

// RedriveQuarantine validates and appends a released envelope atomically,
// exactly once.
func (s *Service) RedriveQuarantine(ctx context.Context, tenantID, eventID, now string) (domain.LogPosition, error) {
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

// redrive revalidates and appends a released event, then marks it redriven.
func (s *Service) redrive(ctx context.Context, u *store.Unit, tenantID, eventID, now string) (domain.LogPosition, error) {
	env, err := u.ReleasedEnvelope(ctx, tenantID, eventID)
	if err != nil {
		return -1, err
	}
	if err := s.admit(ctx, u, tenantID, env); err != nil {
		return -1, fmt.Errorf("validate released envelope: %w", err)
	}
	position, err := s.appendOne(ctx, u, tenantID, env)
	if err != nil {
		return -1, fmt.Errorf("append released event: %w", err)
	}
	return position, u.MarkRedriven(ctx, tenantID, eventID, now)
}
