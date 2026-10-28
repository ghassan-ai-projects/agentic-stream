package domain

import (
	"math"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const (
	CasePending     = "pending"
	CaseVerified    = "verified"
	CaseNotVerified = "not_verified"

	ReasonAwaitingWindow   = "awaiting the settle window"
	ReasonNotAcknowledged  = "the action was not acknowledged"
	ReasonNoSettleWindow   = "the diagnosis code has no settle window"
	ReasonReopened         = "a new situation opened on the entity inside the settle window"
	ReasonNeverClosed      = "the situation did not close inside the settle window"
	ReasonNotRecovered     = "the reading did not return to its level before the action"
	ReasonClosedAndSettled = "closed and stayed closed through the settle window"

	CauseNoEffect = "no_effect"
)

type CaseCandidate struct {
	TenantID         string
	SituationID      string
	SituationType    string
	EntityID         string
	EpisodeID        string
	DecisionID       string
	SituationVersion int
	Decision         map[string]any
	Acknowledged     bool
	Facts            map[string]any
	Evidence         []string
	ActedAt          time.Time
	RuleID           string
	RuleVersion      int
	KindDigest       string
}

type Case struct {
	CaseCandidate
	Observed
	WindowEndsAt time.Time
	Features     map[string]any
	Summary      string
	Verdict      string
	Reason       string
}

type Snapshot struct {
	Facts    map[string]any
	Evidence []string
}

type CaseEvidence struct {
	ClosedAt   *time.Time
	ReopenedAt *time.Time
	Watermark  time.Time
	Closing    Snapshot
}

func (r *Rules) NewCase(candidate CaseCandidate) Case {
	observed := r.Observe(candidate.Decision)
	verified := Case{CaseCandidate: candidate, Observed: observed, Features: r.Features(candidate.Facts, candidate.Evidence),
		Summary: summaryOf(candidate.Decision), Verdict: CasePending, Reason: ReasonAwaitingWindow}
	window, known := r.spec.Promotion.SettleWindow(observed.DiagnosisCode)
	verified.WindowEndsAt = candidate.ActedAt.Add(window)
	switch {
	case !candidate.Acknowledged:
		verified.Verdict, verified.Reason = CaseNotVerified, ReasonNotAcknowledged
	case !known:
		verified.Verdict, verified.Reason = CaseNotVerified, ReasonNoSettleWindow
	}
	return verified
}

func summaryOf(decision map[string]any) string {
	summary, _ := decision["summary"].(string)
	return summary
}

func (r *Rules) Verify(c Case, evidence CaseEvidence) (string, string) {
	return c.Verify(evidence, r.Recovered(c, evidence))
}

func (r *Rules) Recovered(c Case, evidence CaseEvidence) bool {
	check, declared := r.spec.Promotion.RecoveryCheck(c.DiagnosisCode)
	if !declared {
		return true
	}
	return withinTolerance(check, c.Features, r.Features(evidence.Closing.Facts, evidence.Closing.Evidence))
}

func withinTolerance(check spec.Recovery, frozen, closing map[string]any) bool {
	baseline, baselined := number(frozen[check.Baseline])
	reading, read := number(closing[check.Reading])
	return baselined && read && math.Abs(reading-baseline) <= check.Tolerance*math.Abs(baseline)
}

func (c Case) Verify(evidence CaseEvidence, recovered bool) (string, string) {
	switch {
	case evidence.ReopenedAt != nil && !evidence.ReopenedAt.After(c.WindowEndsAt):
		return CaseNotVerified, ReasonReopened
	case evidence.Watermark.Before(c.WindowEndsAt):
		return CasePending, ReasonAwaitingWindow
	case evidence.ClosedAt == nil || evidence.ClosedAt.After(c.WindowEndsAt):
		return CaseNotVerified, ReasonNeverClosed
	case !recovered:
		return CaseNotVerified, ReasonNotRecovered
	default:
		return CaseVerified, ReasonClosedAndSettled
	}
}

func (c Case) RuleHandled() bool { return c.RuleID != "" }

func NoEffectCircuit(c Case, now time.Time) Circuit {
	return Circuit{Rule: RuleKey{ID: c.RuleID, Version: c.RuleVersion}, TenantID: c.TenantID, Cause: CauseNoEffect, OpenedAt: now,
		Evidence: map[string]any{"situation_id": c.SituationID, "episode_id": c.EpisodeID, "reason": c.Reason}}
}
