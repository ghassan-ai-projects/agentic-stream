package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
)

// admit decodes a line, checks the envelope contract, then the registered event
// schema. File replay and the live socket share this rule.
func (s *Service) admit(ctx context.Context, line []byte) domain.LineVerdict {
	verdict := domain.AdmitEnvelope(line, s.tenantID)
	if verdict.Rejected() {
		return verdict
	}
	if err := s.log.ValidateEnvelope(ctx, verdict.Envelope); err != nil {
		return domain.SchemaRejected(verdict.Envelope, err)
	}
	return verdict
}

// quarantineRaw records a refused line as raw bytes.
func (s *Service) quarantineRaw(ctx context.Context, eventID string, line []byte, reason string, now time.Time) error {
	return s.log.QuarantineRaw(ctx, s.tenantID, eventID, line, reason, now) //nolint:wrapcheck // Callers label the failed reason.
}

// quarantineEnvelope records a refused line as its decoded envelope.
func (s *Service) quarantineEnvelope(ctx context.Context, env contractsv1.Envelope, reason string, now time.Time) error {
	return s.log.QuarantineEnvelope(ctx, s.tenantID, env, reason, now) //nolint:wrapcheck // Callers label the failed reason.
}

// quarantined labels a failed quarantine write with its reason.
func quarantined(reason string, err error) error {
	if err != nil {
		return fmt.Errorf("record %s quarantine: %w", reason, err)
	}
	return nil
}
