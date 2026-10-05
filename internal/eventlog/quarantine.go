package eventlog

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Quarantine records an invalid event durably without placing it in the
// executable event log. Repeated delivery increments a bounded retry count.
func (l *EventLog) Quarantine(ctx context.Context, tenantID string, env map[string]any, reason, now string) error {
	return l.service.Quarantine(ctx, tenantID, env, reason, now)
}

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

// RecordGap records a durable discontinuity caused by bounded overflow or
// explicit operator action. It never deletes the original evidence.
func (l *EventLog) RecordGap(ctx context.Context, gapID, tenantID string, partitionID int, fromPosition, toPosition int64, reason, now string) error {
	return l.service.RecordGap(ctx, gapID, tenantID, partitionID, fromPosition, toPosition, reason, now)
}
