package domain

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// MaxPageLimit bounds one delivery page.
const MaxPageLimit = 1000

// PoisonBudget is the number of delivery attempts a malformed notification
// gets before it is skipped and audited.
const PoisonBudget = 3

// Audit actions recorded in notification_audits.
const (
	AuditCursorExpired     = "cursor_expired"
	AuditSubscriberTooSlow = "subscriber_too_slow"
	AuditSubscriberSkipped = "subscriber_skipped"
)

// CheckPageLimit requires a limit between 1 and MaxPageLimit.
func CheckPageLimit(limit int) error {
	if limit <= 0 || limit > MaxPageLimit {
		return fmt.Errorf("notification limit must be between 1 and 1000")
	}
	return nil
}

// Retained is what the outbox still holds for a tenant: the oldest retained
// cursor and the next cursor to allocate.
type Retained struct {
	Oldest        int64
	HasOldest     bool
	NextCursor    int64
	HasNextCursor bool
}

// Refusal is an audited refusal to serve a resume cursor.
type Refusal struct {
	Action  string
	Subject string
	Err     error
}

// RefuseResume decides whether a resume cursor must be refused: it predates
// retained data, or, when maxLag is positive, the subscriber lags too far.
func RefuseResume(cursor, maxLag int64, retained Retained) (Refusal, bool) {
	if retained.expired(cursor) {
		return Refusal{Action: AuditCursorExpired, Subject: "expired cursor", Err: ErrCursorExpired}, true
	}
	if maxLag > 0 && retained.HasNextCursor && retained.NextCursor-1-cursor > maxLag {
		return Refusal{Action: AuditSubscriberTooSlow, Subject: "slow subscriber", Err: ErrSubscriberTooSlow}, true
	}
	return Refusal{}, false
}

func (retained Retained) expired(cursor int64) bool {
	if retained.HasOldest {
		return cursor < retained.Oldest-1
	}
	return retained.HasNextCursor && cursor < retained.NextCursor-1
}

// DecodeRecord verifies the stored digest, then decodes and validates the
// event. It reports false for a poison record.
func DecodeRecord(tenantID string, cursor int64, eventJSON, eventSHA []byte) (Record, bool) {
	if !bytes.Equal(canonicaljson.Sum(eventJSON), eventSHA) {
		return Record{}, false
	}
	record := Record{TenantID: tenantID, Cursor: cursor}
	if err := json.Unmarshal(eventJSON, &record.Event); err != nil {
		return Record{}, false
	}
	if err := record.Event.Validate(); err != nil {
		return Record{}, false
	}
	return record, true
}

// PoisonSpent reports whether a poison record has used its retry budget.
func PoisonSpent(attempts int) bool { return attempts >= PoisonBudget }
