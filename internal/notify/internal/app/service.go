package app

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify/internal/store"
)

// Service runs the notification use cases that own their unit of work: reading
// pages and pruning. Appends join the caller's transaction instead.
type Service struct {
	store      store.Store
	newAuditID func() string
}

// New creates a Service over the store; audit ids are random.
func New(s store.Store) *Service {
	return &Service{store: s, newAuditID: func() string { return ids.Random().New(ids.PrefixAudit) }}
}
