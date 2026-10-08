package eventlog

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// QuarantineEnvelope records a normalized envelope that failed validation.
func (l *EventLog) QuarantineEnvelope(ctx context.Context, tenantID string, env contractsv1.Envelope, reason, now string) error {
	return l.service.QuarantineEnvelope(ctx, tenantID, env, reason, now)
}

// QuarantineRaw preserves malformed JSON as data with a stable line identity.
func (l *EventLog) QuarantineRaw(ctx context.Context, tenantID, eventID string, raw []byte, reason, now string) error {
	return l.service.QuarantineRaw(ctx, tenantID, eventID, raw, reason, now)
}

// ReleaseQuarantine marks one record ready for an explicit re-drive.
func (l *EventLog) ReleaseQuarantine(ctx context.Context, tenantID, eventID, now string) error {
	return l.service.ReleaseQuarantine(ctx, tenantID, eventID, now)
}

// RedriveQuarantine validates and appends a released envelope atomically.
func (l *EventLog) RedriveQuarantine(ctx context.Context, tenantID, eventID, now string) (LogPosition, error) {
	return l.service.RedriveQuarantine(ctx, tenantID, eventID, now)
}
