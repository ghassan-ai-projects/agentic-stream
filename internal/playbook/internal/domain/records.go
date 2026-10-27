package domain

import "time"

const (
	KindShadow = "shadow"
	KindAudit  = "audit"

	CauseAuditDisagreement = "audit_disagreement"
	CauseRateCap           = "rate_cap"
	CauseKindChanged       = "kind_changed"

	firePrefix       = "rfire_"
	evaluationPrefix = "reval_"
	auditPrefix      = "raudit_"
)

type Fire struct {
	FireID           string
	TenantID         string
	EpisodeID        string
	SituationID      string
	SituationVersion int
	Recommendation   Recommendation
	AuditDue         bool
	FiredAt          time.Time
}

type Evaluation struct {
	EvaluationID     string
	TenantID         string
	Kind             string
	FireID           string
	SchedulerItemID  string
	EpisodeID        string
	SituationID      string
	SituationVersion int
	InEnvelope       bool
	InCap            bool
	Recommendation   Recommendation
	Verdict          string
	Observed         *Observed
}

type Circuit struct {
	Rule     RuleKey
	TenantID string
	Cause    string
	Evidence map[string]any
	OpenedAt time.Time
}

func NewFire(tenantID, episodeID, situationID string, version int, recommendation Recommendation, auditEvery int, now time.Time) Fire {
	return Fire{FireID: firePrefix + episodeID, TenantID: tenantID, EpisodeID: episodeID, SituationID: situationID,
		SituationVersion: version, Recommendation: recommendation, AuditDue: AuditDue(situationID, version, auditEvery), FiredAt: now}
}

func NewShadowEvaluation(tenantID, episodeID, situationID string, version int, match Match, recommendation Recommendation) Evaluation {
	return Evaluation{EvaluationID: evaluationPrefix + episodeID + "_" + recommendation.RuleID, TenantID: tenantID, Kind: KindShadow,
		EpisodeID: episodeID, SituationID: situationID, SituationVersion: version, InEnvelope: match.InEnvelope, InCap: match.InCap,
		Recommendation: recommendation, Verdict: VerdictPending}
}

func NewAuditEvaluation(fire Fire, schedulerItemID string) Evaluation {
	return Evaluation{EvaluationID: auditPrefix + fire.FireID, TenantID: fire.TenantID, Kind: KindAudit, FireID: fire.FireID,
		SchedulerItemID: schedulerItemID, SituationID: fire.SituationID, SituationVersion: fire.SituationVersion,
		InEnvelope: true, InCap: true, Recommendation: fire.Recommendation, Verdict: VerdictPending}
}

func AuditKey(fire Fire) string {
	return auditPrefix + fire.FireID
}

func RateCapCircuit(tenantID string, match Match, fires int, now time.Time) Circuit {
	return Circuit{Rule: keyOf(match.Rule), TenantID: tenantID, Cause: CauseRateCap, OpenedAt: now,
		Evidence: map[string]any{"fires_last_hour": fires, "rate_cap_per_hour": match.Rule.RateCapPerHour}}
}

func DisagreementCircuit(evaluation Evaluation, observed Observed, now time.Time) Circuit {
	return Circuit{Rule: evaluation.Recommendation.Key(), TenantID: evaluation.TenantID, Cause: CauseAuditDisagreement, OpenedAt: now,
		Evidence: map[string]any{"episode_id": evaluation.EpisodeID, "situation_id": evaluation.SituationID,
			"situation_version": evaluation.SituationVersion, "recommended": evaluation.Recommendation, "observed": observed}}
}

func (r *Rules) KindChanges(tenantID string, born map[string]string, now time.Time) []Circuit {
	var circuits []Circuit
	for _, compiled := range r.compiled {
		pinned := compiled.rule.PinnedKind
		if digest, seen := born[pinned.Name]; seen && digest != pinned.Digest {
			circuits = append(circuits, Circuit{Rule: keyOf(compiled.rule), TenantID: tenantID, Cause: CauseKindChanged, OpenedAt: now,
				Evidence: map[string]any{"kind": pinned.Name, "pinned_digest": pinned.Digest, "born_digest": digest}})
		}
	}
	return circuits
}
