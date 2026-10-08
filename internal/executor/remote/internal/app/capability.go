package app

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
)

// AttemptCapabilityIssuer derives one short-lived evidence capability from a
// trusted durable Request. The raw token exists only in the dispatch call.
type AttemptCapabilityIssuer struct {
	Issuer       *evidence.Service
	RuntimeEpoch string
	Tools        []string
	From         time.Time
	Until        time.Time
	MaxRows      uint64
	MaxBytes     uint64
	ExpiresAt    time.Time
}

// Issue creates a capability bound to the exact attempt identity and request
// trace. It fails closed when any required scope is absent.
func (i *AttemptCapabilityIssuer) Issue(req *episodes.Request) ([]byte, error) {
	if i == nil || i.Issuer == nil || req == nil {
		return nil, fmt.Errorf("attempt capability issuer is not configured")
	}
	if req.EpisodeID == "" || req.AttemptID == "" || req.Fence <= 0 || req.TenantID == "" || req.SituationID == "" || req.SituationVersion <= 0 || req.EntityID == "" || i.RuntimeEpoch == "" || len(i.Tools) == 0 || i.From.IsZero() || i.Until.IsZero() || i.MaxRows == 0 || i.MaxBytes == 0 {
		return nil, fmt.Errorf("attempt capability scope is incomplete")
	}
	return i.signAttemptScope(req, i.Issuer.IssueTime())
}

func (i *AttemptCapabilityIssuer) signAttemptScope(req *episodes.Request, now time.Time) ([]byte, error) {
	token, err := i.Issuer.Issue(evidence.Scope{
		EpisodeID: req.EpisodeID, AttemptID: req.AttemptID, Fence: req.Fence,
		TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: int64(req.SituationVersion), EntityID: req.EntityID,
		Tools: append([]string(nil), i.Tools...), NotBefore: now, ExpiresAt: i.ExpiresAt,
		MaxRows: i.MaxRows, MaxBytes: i.MaxBytes, From: i.From, Until: i.Until,
		Traceparent: req.Traceparent, Tracestate: req.Tracestate, RuntimeEpoch: i.RuntimeEpoch,
	})
	if err != nil {
		return nil, fmt.Errorf("issue attempt capability: %w", err)
	}
	return token, nil
}
