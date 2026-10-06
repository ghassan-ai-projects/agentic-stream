package evidence

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Scope is the complete bounded authorization scope.
type Scope = domain.Scope

// Call is an admitted application query.
type Call = domain.Call

// Query is the read-only provider port.
type Query = domain.Query

// QueryResult holds exact bounded response bytes.
type QueryResult = domain.QueryResult

// EvidenceGetArguments is the closed v1 argument record.
type EvidenceGetArguments = domain.EvidenceGetArguments

// NewRuntimeEpoch generates one opaque process-owner identity.
func NewRuntimeEpoch() (string, error) { return wire.NewRuntimeEpoch() }

// EventLogQuery reads evidence through the eventlog owner's scoped API.
func EventLogQuery(db *storage.DB) Query { return transport.EventLogQuery(db) }
