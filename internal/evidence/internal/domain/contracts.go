// Package domain defines evidence authorization and reservation rules over values.
package domain

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Scope is the complete authorization scope of one short-lived capability.
// It is intentionally bound to the durable worker identity and cannot grant
// access to effectors, credentials, filesystem, shell, or arbitrary network.
type Scope struct {
	KeyID            string
	Issuer           string
	Audience         string
	EpisodeID        string
	AttemptID        string
	Fence            int64
	TenantID         string
	SituationID      string
	SituationVersion int64
	EntityID         string
	TokenID          string
	IssuedAt         time.Time
	Tools            []string
	NotBefore        time.Time
	ExpiresAt        time.Time
	MaxRows          uint64
	MaxBytes         uint64
	From             time.Time
	Until            time.Time
	Traceparent      string
	Tracestate       string
	RuntimeEpoch     string
}

// Query receives an already-authorized, bounded evidence request. It must not
// expose database handles, credentials, shell access, arbitrary HTTP, or
// effectors to the worker.
type Query func(context.Context, Call) (QueryResult, error)

// QueryResult is the bounded, typed output of a runtime-owned evidence
// provider.
type QueryResult struct {
	JSON     []byte
	RowCount uint64
}

func (r QueryResult) SHA256() []byte {
	sum := sha256.Sum256(r.JSON)
	return sum[:]
}

// EvidenceGetArguments is the complete v1 schema for evidence.get. Query
// dimensions and limits are authenticated protobuf fields, not worker-owned
// JSON fields.
type EvidenceGetArguments struct {
	EntityID string `json:"entity_id"`
}

// Call is the validated application form of an EvidenceToolCall.
type Call struct {
	EpisodeID        string
	CallID           string
	ToolName         string
	TenantID         string
	SituationID      string
	SituationVersion int64
	EntityID         string
	Arguments        EvidenceGetArguments
	Deadline         time.Time
	AttemptID        string
	Fence            int64
	Trace            contractsv1.TraceContext
	MaxRows          uint64
	MaxBytes         uint64
	From             time.Time
	Until            time.Time
}
