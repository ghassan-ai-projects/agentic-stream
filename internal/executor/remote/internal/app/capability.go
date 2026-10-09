package app

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
)

type AttemptCapabilityIssuer struct {
	Issuer       *evidence.Service
	RuntimeEpoch string
	Tools        []string
	Window       time.Duration
	MaxRows      uint64
	MaxBytes     uint64
	ExpiresAt    time.Time
}

type EvidenceGrant struct {
	Token       []byte
	From, Until time.Time
}

func (i *AttemptCapabilityIssuer) Issue(req *episodes.Request) (EvidenceGrant, error) {
	if i == nil || i.Issuer == nil || req == nil {
		return EvidenceGrant{}, fmt.Errorf("attempt capability issuer is not configured")
	}
	if req.EpisodeID == "" || req.AttemptID == "" || req.Fence <= 0 || req.TenantID == "" || req.SituationID == "" || req.SituationVersion <= 0 || req.EntityID == "" || i.RuntimeEpoch == "" || len(i.Tools) == 0 || i.Window <= 0 || i.MaxRows == 0 || i.MaxBytes == 0 {
		return EvidenceGrant{}, fmt.Errorf("attempt capability scope is incomplete")
	}
	horizon, err := req.EvidenceHorizon()
	if err != nil {
		return EvidenceGrant{}, fmt.Errorf("attempt capability window: %w", err)
	}
	return i.signAttemptScope(req, EvidenceGrant{From: horizon.Add(-i.Window), Until: horizon}, i.Issuer.IssueTime())
}

func (i *AttemptCapabilityIssuer) signAttemptScope(req *episodes.Request, grant EvidenceGrant, now time.Time) (EvidenceGrant, error) {
	token, err := i.Issuer.Issue(evidence.Scope{
		EpisodeID: req.EpisodeID, AttemptID: req.AttemptID, Fence: req.Fence,
		TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: int64(req.SituationVersion), EntityID: req.EntityID,
		Tools: append([]string(nil), i.Tools...), NotBefore: now, ExpiresAt: i.ExpiresAt,
		MaxRows: i.MaxRows, MaxBytes: i.MaxBytes, From: grant.From, Until: grant.Until,
		Traceparent: req.Traceparent, Tracestate: req.Tracestate, RuntimeEpoch: i.RuntimeEpoch,
	})
	if err != nil {
		return EvidenceGrant{}, fmt.Errorf("issue attempt capability: %w", err)
	}
	grant.Token = token
	return grant, nil
}
