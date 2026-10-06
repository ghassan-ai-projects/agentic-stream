package domain

import (
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// ErrCursorExpired means the requested cursor is older than retained data;
// the caller must perform an audited resnapshot before resuming.
var ErrCursorExpired = errors.New("notification cursor expired")

// ErrSubscriberTooSlow means the subscriber exceeded its bounded backlog.
var ErrSubscriberTooSlow = errors.New("subscriber too slow")

// ErrNotificationPoison means a malformed notification is waiting for a
// bounded redelivery attempt. Once the durable retry limit is reached, the
// row is skipped and audited.
var ErrNotificationPoison = errors.New("notification poison pending retry")

// ErrEventExpired means an event older than the notification deduplication
// horizon cannot be reintroduced after retention pruning.
var ErrEventExpired = errors.New("notification event expired")

// Record is one durable notification and its tenant-local cursor.
type Record struct {
	TenantID string
	Cursor   int64
	Event    contractsv1.CloudEvent
}

// Page is a bounded delivery result. NextCursor advances past skipped poison
// rows so a consumer cannot stall forever on one corrupt record.
type Page struct {
	Records    []Record
	NextCursor int64
	Skipped    int
}

// PageRequest asks for up to Limit records strictly after Cursor. A positive
// MaxLag disconnects a subscriber lagging further behind.
type PageRequest struct {
	TenantID string
	Cursor   int64
	Limit    int
	MaxLag   int64
}

// Deliver advances the page past cursor and appends a valid record.
func (page *Page) Deliver(record Record) {
	page.NextCursor = record.Cursor
	page.Records = append(page.Records, record)
}

// Skip advances the page past a poison cursor whose retry budget is spent.
func (page *Page) Skip(cursor int64) {
	page.NextCursor = cursor
	page.Skipped++
}
